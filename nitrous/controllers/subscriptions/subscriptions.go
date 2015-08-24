package subscriptions

import (
	"database/sql"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
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

	errCannotUpgrade        = httperror.New(http.StatusBadRequest, "invalid_request", "you cannnot upgrade to this plan")
	errOrgForbidden         = httperror.New(http.StatusForbidden, "org_forbidden", "you do not have access to this organization")
	errCannotReactivate     = httperror.New(http.StatusBadRequest, "invalid_request", "you cannot reactivate a subscription that has not been cancelled")
	errSubscriptionNotFound = httperror.New(http.StatusNotFound, "subscription_not_found", "subscription does not exist")
)

func init() {
	subProvider = sub.NewProvider()
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	var (
		err           error
		subscriptions []*subscription.SubscriptionWithHostSlug
	)
	if orgSlug := mux.Vars(r)["org_slug"]; orgSlug != "" {
		mem, err := orgmembership.FindByUsernameAndOrgSlug(env.DB, env.User.Username, orgSlug)
		if err != nil {
			if err == sql.ErrNoRows {
				return errOrgForbidden
			}
			return err
		}
		if mem.Role != orgmembership.RoleOwner {
			return errOrgForbidden
		}
		subscriptions, err = subscription.FindAllActiveForOrg(env.DB, mem.OrgID)
	} else {
		subscriptions, err = subscription.FindAllActiveByUserID(env.DB, env.User.ID)
	}
	if err != nil {
		return err
	}

	plans, err := fetchPlansForSubscription(env.DB, subscriptions)
	if err != nil {
		return err
	}
	serializedSubscriptions := []*serializers.Subscription{}

	for _, s := range subscriptions {
		serializedSubscription := serializers.SerializeSubscription(s)
		serializedSubscription.Plan = plans[s.PlanID]
		serializedSubscriptions = append(serializedSubscriptions, serializedSubscription)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"subscriptions": serializedSubscriptions,
	}
	return nil
}

func UpgradeEstimate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	slug := mux.Vars(r)["slug"]

	s, err := subscription.FindBySlug(env.DB, slug, env.User.ID)
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
	slug := mux.Vars(r)["slug"]

	s, err := subscription.FindBySlug(env.DB, slug, env.User.ID)
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
	slug := mux.Vars(r)["slug"]

	s, err := subscription.FindBySlug(env.DB, slug, env.User.ID)
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

func Reactivate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	slug := mux.Vars(r)["slug"]

	s, err := subscription.FindBySlug(env.DB, slug, env.User.ID)
	if err != nil {
		return errSubscriptionNotFound
	}

	if !s.CancelPending {
		return errCannotReactivate
	}

	err = subProvider.Reactivate(env.DB, s)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "reactivated",
	}
	return nil
}

func fetchPlansForSubscription(d db.QueryReceiver, subs []*subscription.SubscriptionWithHostSlug) (map[int64]*serializers.Plan, error) {
	planMap := map[int64]*serializers.Plan{}

	if len(subs) == 0 {
		return planMap, nil
	}

	ids := make([]int64, len(subs))
	for i, sub := range subs {
		ids[i] = sub.PlanID
	}

	query, args, err := sqlx.In("SELECT * FROM plans WHERE id IN (?);", ids)
	if err != nil {
		return nil, err
	}

	plans := []*plan.Plan{}

	query = d.Rebind(query)
	err = d.Select(&plans, query, args...)
	if err != nil {
		return planMap, err
	}

	for _, pl := range plans {
		ser := serializers.SerializePlan(pl)
		planMap[pl.ID] = ser
	}

	return planMap, nil
}
