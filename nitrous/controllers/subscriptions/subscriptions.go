package subscriptions

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	sub "github.com/nitrous-io/dedicated/nitrous/subscriptions"
	stripe "github.com/stripe/stripe-go"
	stripeinvoice "github.com/stripe/stripe-go/invoice"
)

var (
	subProvider sub.Provider

	errCannotUpgrade = httperror.New(http.StatusBadRequest, "invalid_request", "you cannnot upgrade to this plan")
)

func init() {
	subProvider = sub.NewProvider()
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	subscriptions, err := subscription.FindAllActiveByUserID(env.DB, env.User.ID)
	if err != nil {
		return err
	}
	serializedSubscriptions := []*serializers.Subscription{}

	for _, s := range subscriptions {
		serializedSubscription := serializers.SerializeSubscription(s)
		serializedSubscriptions = append(serializedSubscriptions, serializedSubscription)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"subscriptions": serializedSubscriptions,
	}
	return nil
}

func UpgradeEstimate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		return err
	}

	s, err := subscription.FindByUserID(env.DB, id, env.User.ID)
	if err != nil {
		return err
	}

	planID, err := strconv.ParseInt(r.FormValue("plan_id"), 10, 64)
	if err != nil {
		return err
	}

	pl, err := plan.FindByID(env.DB, planID)
	if err != nil {
		return err
	}

	bp, err := env.User.BillingProfile(env.DB)
	if err != nil {
		return err
	}

	stripePlanID := pl.StripePlanID.String
	if bp.IsExempt() {
		stripePlanID = stripePlanID + "-exempt"
	}

	// Stripe's InvoiceParams struct prevents sending `subscription_plan` to see prorata estimates
	// so we build the request ourselves
	body := &url.Values{
		"customer":             []string{bp.StripeID.String},
		"subscription":         []string{s.StripeID.String},
		"subscription_plan":    []string{stripePlanID},
		"subscription_prorate": []string{"true"},
	}

	c := &stripeinvoice.Client{stripe.GetBackend(), stripe.Key}
	i := &stripe.Invoice{}
	err = c.B.Call("GET", "/invoices/upcoming", c.Key, body, i)

	if err != nil {
		return err
	}

	var amount int64
	for _, l := range i.Lines.Values {
		if l.Proration {
			amount += l.Amount
		}
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"total": amount,
	}
	return nil
}

func Upgrade(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		return err
	}

	s, err := subscription.FindByUserID(env.DB, id, env.User.ID)
	if err != nil {
		return err
	}

	planID, err := strconv.ParseInt(r.FormValue("plan_id"), 10, 64)
	if err != nil {
		return err
	}

	pl, err := plan.FindByID(env.DB, planID)
	if err != nil {
		return err
	}

	err = subProvider.Upgrade(env.DB, s, pl, env.User)
	if err != nil {
		if err == sub.ErrCannotUpgrade {
			return errCannotUpgrade
		}
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"subscription": "upgraded",
	}
	return nil
}

func Cancel(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		return err
	}

	s, err := subscription.FindByUserID(env.DB, id, env.User.ID)
	if err != nil {
		return err
	}

	err = subProvider.Cancel(env.DB, s)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "cancelled",
	}
	return nil
}
