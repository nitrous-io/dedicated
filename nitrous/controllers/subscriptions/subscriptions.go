package subscriptions

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/controllers/shared"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
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

	errCannotCancel         = httperror.New(http.StatusBadRequest, "invalid_request", "you cannot cancel a subscription that has been canceled")
	errCannotUpgrade        = httperror.New(http.StatusBadRequest, "invalid_request", "you cannnot upgrade to this plan")
	errCannotReactivate     = httperror.New(http.StatusBadRequest, "invalid_request", "you cannot reactivate a subscription that has not been cancelled")
	errOrgForbidden         = httperror.New(http.StatusForbidden, "org_forbidden", "you do not have access to this organization")
	errSubscriptionNotFound = httperror.New(http.StatusNotFound, "subscription_not_found", "subscription does not exist")
	errOwnerOnly            = httperror.New(http.StatusForbidden, "owner_only", "only a org owner can modify this subscription")

	errPlanEmpty    = httperror.New(http.StatusBadRequest, "plan_not_provided", "`plan` must be provided")
	errPlanNotFound = httperror.New(http.StatusBadRequest, "plan_not_found", "the specified `plan` does not exist")

	errOrgOnly = httperror.New(http.StatusForbidden, "org_only", "only orgs can use this endpoint at this time")

	errMultipleSubscriptionNotAllowed = httperror.New(http.StatusBadRequest, "invalid_request", "you can only perform one subscribe at a time")
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

func Subscribe(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// NOTE: for now this is locked to support orgs only
	// modify this endpoint to support additional types (users)

	db := env.DB

	planSlug := r.FormValue("plan")
	if planSlug == "" {
		return errPlanEmpty
	}
	pl, err := plan.FindBySlug(db, planSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errPlanNotFound
		}
		return err
	}

	bp, err := shared.BillingProfileForRequest(env, r)
	if err != nil {
		return err
	}

	var o *org.Org
	if !bp.OrgID.Valid {
		return errOrgOnly
	} else {
		o, err = org.Find(db, bp.OrgID.Int64)
		if err != nil {
			return err
		}
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	defer func() {
		tx.Rollback()
	}()

	locked, err := ndb.TransactionLock(tx, ndb.SUBSCRIBE_LOCK, int(bp.ID))
	if err != nil {
		return err
	}
	if !locked {
		return errMultipleSubscriptionNotAllowed
	}

	rSub, err := subProvider.Subscribe(db, nil, pl, env.User, o)
	if err != nil {
		return shared.HandleBillingError(err)
	}

	s := &subscription.SubscriptionWithHostSlug{
		Subscription: rSub,
	}
	if o != nil {
		s.OrgID = sql.NullInt64{o.ID, true}
	}

	ser := serializers.SerializeSubscription(s)
	ser.Plan = serializers.SerializePlan(pl)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"subscription": ser,
	}
	return nil
}

func UpgradeEstimate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	s, err := fetchSubscriptionForWriting(env, r)
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

	bp, err := billingprofile.FindByID(env.DB, s.BillingProfileID)
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
	s, err := fetchSubscriptionForWriting(env, r)
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
	s, err := fetchSubscriptionForWriting(env, r)
	if err != nil {
		return err
	}

	if s.AlreadyCanceled() {
		return errCannotCancel
	}

	if err := subProvider.Cancel(env.DB, s); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "cancelled",
	}
	return nil
}

func Reactivate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	s, err := fetchSubscriptionForWriting(env, r)
	if err != nil {
		return err
	}

	if !s.CancelPending {
		return errCannotReactivate
	}

	if !s.MultiHost {

		if _, err := host.FindBySubscriptionID(env.DB, s.ID); err != nil {
			return errSubscriptionNotFound
		}
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

func Hosts(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	s, err := fetchSubscriptionForWriting(env, r)
	if err != nil {
		return err
	}

	hosts, err := host.FindAllBySubscriptionID(env.DB, s.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hosts": serializers.SerializeUserHosts(hosts),
	}
	return nil
}

// multi-host subscriptions should only be modifed by an owner of the Billing Profile (or Org)
// so we still allow users to view susbcriptions they depend on (they may have a host)
// but we prevent any modification of a subscription unless they "own" it
func fetchSubscriptionForWriting(env *middleware.Env, r *http.Request) (*subscription.Subscription, error) {
	slug := mux.Vars(r)["slug"]

	s, err := subscription.FindBySlug(env.DB, slug, env.User.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errSubscriptionNotFound
		}
		return nil, err
	}

	// we are only checking multi-host for permision, other subscriptions have existing control
	if !s.MultiHost {
		return s, nil
	}

	bp, err := billingprofile.FindByID(env.DB, s.BillingProfileID)
	if err != nil {
		return nil, err
	}

	if bp.UserID == env.User.ID {
		return s, nil
	}

	// perform sanity check
	if !bp.OrgID.Valid {
		return nil, fmt.Errorf("Expected OrgID to be set on billing profile %d", bp.ID)
	}

	mem, err := orgmembership.FindByUserIDAndOrgID(env.DB, env.User.ID, bp.OrgID.Int64)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errSubscriptionNotFound
		}
		return nil, err
	}

	if mem.Role != orgmembership.RoleOwner {
		return nil, errOwnerOnly
	}

	return s, nil
}

func fetchPlansForSubscription(d ndb.QueryReceiver, subs []*subscription.SubscriptionWithHostSlug) (map[int64]*serializers.Plan, error) {
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
