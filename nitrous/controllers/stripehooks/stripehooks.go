package stripehooks

import (
	"encoding/json"
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/stripe/stripe-go/event"
)

type IncomingStripeWebHook struct {
	EventID string `json:"id"`
	Type    string `json:"type"`
}

var (
	HandlePaymentSucceededFn func(*middleware.Env, *invoice.Invoice)            = handlePaymentSucceeded
	HandlePaymentFailedFn    func(*middleware.Env, *invoice.Invoice, int, bool) = handlePaymentFailed
)

func ReceiveHook(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	evt := IncomingStripeWebHook{}
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		return err
	}
	go func() {
		handleStripeWebhook(env, evt)
	}()
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func handleStripeWebhook(env *middleware.Env, evt IncomingStripeWebHook) {
	// verify the event is legit
	env.Logger.Debugf("Incoming event: %s", evt.Type)
	if !(evt.Type == "invoice.payment_succeeded" || evt.Type == "invoice.payment_failed") {
		return
	}
	e, err := event.Get(evt.EventID)
	if err != nil {
		env.Logger.Errorf("Stripe hook event lookup failed, err: %v", err)
	}

	if e.Data == nil {
		env.Logger.Error("Stripe hook event is missing Data: %+v", e)
		return
	}

	stripeID, ok := e.Data.Obj["id"].(string)
	if !ok {
		env.Logger.Errorf("Stripe failed to provide id: %+v", e.Data.Obj)
		return
	}
	inv, err := invoice.FindByStripeID(env.DB, stripeID)
	if err != nil {
		env.Logger.Errorf("Error when locating invoice %s, err: %v", stripeID, err)
		return
	}
	if inv == nil {
		env.Logger.Errorf("Could not find invoice matching Stripe webhook %+v, event: %+v", evt, e)
		return
	}

	if evt.Type == "invoice.payment_succeeded" {
		HandlePaymentSucceededFn(env, inv)
	} else {
		att, ok := e.Data.Obj["attempt_count"].(float64)
		if !ok {
			env.Logger.Errorf("Stripe failed to provide attempt_count: %+v", e.Data.Obj)
			return
		}
		nxt_attempt, ok := e.Data.Obj["next_payment_attempt"].(float64)
		HandlePaymentFailedFn(env, inv, int(att), ok && nxt_attempt >= 0)
	}
}

func handlePaymentSucceeded(env *middleware.Env, inv *invoice.Invoice) {
	if err := inv.PaymentSucceeded(env.DB); err != nil {
		env.Logger.Errorf("Error marking invoice %d as succeeded, err: %v", inv.ID, err)
	}
}

func handlePaymentFailed(env *middleware.Env, inv *invoice.Invoice, attemptCount int, moreAttempts bool) {
	if err := inv.PaymentFailed(env.DB, attemptCount, moreAttempts); err != nil {
		env.Logger.Errorf("Error marking invoice %d as failed, err: %v", inv.ID, err)
	}
}
