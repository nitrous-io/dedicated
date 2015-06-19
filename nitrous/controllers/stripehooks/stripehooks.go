package stripehooks

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/event"
)

type IncomingStripeWebHook struct {
	EventID string `json:"id"`
	Type    string `json:"type"`
}

type EventHandlerFn func(env *middleware.Env, e *stripe.Event, stripeID string) error

var (
	evtHandlers = map[string]EventHandlerFn{
		"invoice.payment_succeeded":     handleStripeInvoiceEvent,
		"invoice.payment_failed":        handleStripeInvoiceEvent,
		"customer.subscription.updated": handleSubscriptionUpdatedEvent,
		"customer.subscription.deleted": handleSubscriptionDeletedEvent,
		"invoice.created":               handleInvoiceCreatedEvent,
	}
)

func ReceiveHook(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	evt := IncomingStripeWebHook{}
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		return err
	}
	if err := handleStripeWebhook(env, evt); err != nil {
		return err
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func handleStripeWebhook(env *middleware.Env, evt IncomingStripeWebHook) error {
	// verify the event is legit
	logrus.Debugf("Incoming event: %s", evt.Type)
	f, ok := evtHandlers[evt.Type]
	if !ok {
		return nil
	}
	e, err := event.Get(evt.EventID)
	if err != nil {
		return fmt.Errorf("Stripe hook event lookup failed, err: %v", err)
	}

	if e.Data == nil {
		return fmt.Errorf("Stripe hook event is missing Data: %+v", e)
	}

	stripeID, ok := e.Data.Obj["id"].(string)
	if !ok {
		return fmt.Errorf("Stripe failed to provide id: %+v", e.Data.Obj)
	}

	env.Logger.Info("Processing Stripe event: %+v", e)
	return f(env, e, stripeID)
}
