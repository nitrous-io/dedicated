package subscriptions

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/coupon"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/respec"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	stripe "github.com/stripe/stripe-go"
	stripecoupon "github.com/stripe/stripe-go/coupon"
	stripeinvoice "github.com/stripe/stripe-go/invoice"
	stripeinvoiceitem "github.com/stripe/stripe-go/invoiceitem"
	stripesub "github.com/stripe/stripe-go/sub"
)

type SubscribeOptions struct {
	Host         *host.Host
	Plan         *plan.Plan
	User         *user.User
	Org          *org.Org
	PromoCode    *promocode.PromoCode
	PromoCodeUse *promocode.PromoCodeUse
}

type Provider interface {
	Subscribe(d *db.Database, opts *SubscribeOptions) (*subscription.Subscription, error)
	Cancel(d *db.Database, sub *subscription.Subscription) error
	Upgrade(d *db.Database, sub *subscription.Subscription, p *plan.Plan, u *user.User) error
	Reactivate(d *db.Database, sub *subscription.Subscription) error

	HandleSubscriptionUpdated(d *db.Database, subID string) (*subscription.Subscription, error)
	HandleSubscriptionDeleted(d *db.Database, subID string) (*subscription.Subscription, error)
	HandleSubscriptionInvoiceCreated(d *db.Database, invID string) (*invoice.Invoice, error)
	HandleSubscriptionInvoiceFailed(d *db.Database, invID string) (*invoice.Invoice, error)

	HandleChargeSucceeded(d *db.Database, e *stripe.Event) error
	HandleChargeFailed(d *db.Database, e *stripe.Event) error
}

type couponGetFunc func(string, *stripe.CouponParams) (*stripe.Coupon, error)

type hostStopFunc func(db *db.Database, h *host.Host, headers http.Header, ev hostcontrol.EventFunc, initiatorID int64) error

type SubscriptionProvider struct {
	HostStopFunc hostStopFunc
	Mailer       mailer.Mailer
}

const (
	compHostingModeCouponSlug = "comped-hosting-mode"
)

var (
	couponGetFn couponGetFunc

	ErrOrgPlanProvided = errors.New("A org plan was provided but no org was provided")
	ErrPlanNotOrg      = errors.New("The provided plan is not for orgs")
	ErrPlanNotEdu      = errors.New("The edu state of the plan and org do not match")

	ErrNotSubscribablePlan   = errors.New("Plan does not support subscriptions")
	ErrInvalidBillingState   = errors.New("Invalid billing state")
	ErrPlanNotFree           = errors.New("Plan not free but billing state in Free")
	ErrInvoiceAlreadyCreated = errors.New("Invoice already exists")

	ErrInvoiceLackingSubscription = errors.New("Invoice not attached to Subscription")
	ErrInvoiceAlreadyPaid         = errors.New("Invoice has already been paid")

	ErrCardExpired  = errors.New("Your Card has expired. Please try again with a recent card.")
	ErrCardDeclined = errors.New("Your Card was declined. Try contacting the bank directly for assistance.")

	ErrCannotUpgrade = errors.New("You cannot upgrade to this plan")

	ErrHostNotProvided = errors.New("You must provide a host when subscribing to a single host plan")

	ErrExistingMultiHostSubscription = errors.New("You can only have one multi-host subscription at a time")
)

type updatedAvailableCredits struct {
	Consumed  int64
	Available int64
}

func init() {
	couponGetFn = stripecoupon.Get
}

func NewProvider() *SubscriptionProvider {
	s := &SubscriptionProvider{}
	s.HostStopFunc = hostcontrol.StopWithQueue
	s.Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)
	return s
}

func (s *SubscriptionProvider) Subscribe(d *db.Database, opts *SubscribeOptions) (sub *subscription.Subscription, err error) {
	h, p := opts.Host, opts.Plan
	u, o := opts.User, opts.Org
	pc, pcUse := opts.PromoCode, opts.PromoCodeUse

	if !opts.Plan.IsSubscription() {
		err = ErrNotSubscribablePlan
		return
	}

	if err = canSubscribe(u, o, p); err != nil {
		return
	}

	var bp *billingprofile.BillingProfile
	if o != nil {
		bp, err = billingprofile.FindForOrg(d, o.ID)
	} else {
		bp, err = u.BillingProfile(d)
	}
	if err != nil {
		return
	}

	if !bp.StripeID.Valid {
		if o != nil {
			err = o.ActivateBillingProfile(d, bp, "")
		} else {
			bp, err = u.ActivateBillingProfile(d, "")
		}
		if err != nil {
			return
		}
	}

	if !bp.EligibleForSubscription() {
		err = ErrInvalidBillingState
		return
	}

	if (!p.MaxHosts.Valid || p.MaxHosts.Int64 == 1) && h == nil {
		err = ErrHostNotProvided
		return
	}

	if p.MaxHosts.Valid {
		// verify user does not have an existing multi-host plan (enforcing one per BP for now)
		_, err = subscription.FindMultiHostSubscription(d, bp.ID)
		if err != sql.ErrNoRows {
			if err == nil {
				err = ErrExistingMultiHostSubscription
			}
			return
		}
		err = nil
	}

	if pcUse == nil && pc != nil {
		pcUse, err = promocode.Claim(d, pc.Slug, bp.ID)
		if err != nil {
			logrus.Errorf("Unable to claim promocode %s for user %d, err: %v", pc.Slug, u.ID, err)
			pc = nil
		}
	}

	var strItem *stripe.InvoiceItem
	var strSub *stripe.Sub

	err = d.WithTransaction(func(tx *sqlx.Tx) error {
		var err error

		slug, err := securerandom.Base64(8)
		if err != nil {
			return err
		}
		sub = &subscription.Subscription{
			PlanID:           p.ID,
			BillingProfileID: bp.ID,
			State:            subscription.StateCreating,
			Slug:             slug[:10],
			MultiHost:        p.MaxHosts.Valid,
		}
		if err := sub.Insert(tx); err != nil {
			return err
		}

		strItem, err = creditStripeInvoice(tx, bp, p.Price, sub.ID, p.StripePlanID.String, nil)
		if err != nil {
			return err
		}

		planID := p.StripePlanID.String
		if bp.State == billingprofile.StateExempt {
			planID = planID + "-exempt"
		}

		params := &stripe.SubParams{
			Customer: bp.StripeID.String,
			Plan:     planID,
		}

		if pc != nil && pc.TrialSeconds.Valid {
			params.TrialEnd = time.Now().Unix() + pc.TrialSeconds.Int64
		} else if trialEnd := p.TrialEnd(time.Now().Unix()); trialEnd > 0 && bp.TrialEligible {
			params.TrialEnd = trialEnd
		}

		var cp *coupon.Coupon
		if !p.IsFree() {
			cp, err = coupon.Find(tx, u.ID)
			if err == nil {
				code := cp.CodeForStripe()
				if _, err := couponGetFn(code, nil); err != nil {
					// log and ignore attempt to use invalid coupon
					logrus.Warnf("Attempted to use invalid coupon (username, code): (%s, %s)", u.Username, code)
				} else {
					params.Coupon = code
				}
			}
		}

		strSub, err = stripesub.New(params)
		if err != nil {
			logrus.Errorf("Failed to create Stripe subscription: %+v, err: %+v", strSub, err)
			strSub = nil

			return decodeStripeError(err)
		}

		// If we used a coupon, mark it as used.
		if cp != nil && params.Coupon != "" {
			if err := cp.MarkAsUsed(tx); err != nil {
				return err
			}
		}

		sub.State = string(strSub.Status)
		sub.StripeID = sql.NullString{string(strSub.ID), true}
		sub.CurrentPeriodStart = pq.NullTime{time.Unix(strSub.PeriodStart, 0), true}
		sub.CurrentPeriodEnd = pq.NullTime{time.Unix(strSub.PeriodEnd, 0), true}
		if strSub.TrialEnd != 0 {
			sub.TrialEnd = pq.NullTime{time.Unix(strSub.TrialEnd, 0), true}
		}

		nstmt, err := tx.PrepareNamed(`UPDATE subscriptions
			SET state = :state, stripe_id = :stripe_id,
			current_period_start = :current_period_start,
			current_period_end = :current_period_end,
			trial_end = :trial_end,
			updated_at = now()
			WHERE id = :id
			RETURNING *;`)
		if err != nil {
			return err
		}
		if err := nstmt.QueryRow(sub).StructScan(sub); err != nil {
			return err
		}

		bp.TrialEligible = false
		if err := bp.Update(tx); err != nil {
			logrus.Errorf("bp.Update: Failed to update billing profile trial_eligible on new subscription: err: %v", strSub, err)
			return err
		}

		// skip if this is a multi host plan
		if h == nil {
			return nil
		}

		return tx.QueryRowx(`UPDATE hosts
                SET (subscription_id, idle_notices_sent, idle_next_check, updated_at) =
                ($1, $2, NULL, now())
                WHERE id = $3 RETURNING *;`, sql.NullInt64{sub.ID, true}, host.IDLE_NOTICES_NO_WARNING, h.ID).StructScan(h)
	})

	if err != nil {
		sub = nil
		if strItem != nil {
			logrus.Errorf("Subscribe failed, deleting Stripe invoice item %s", strItem.ID)
			if err := stripeinvoiceitem.Del(strItem.ID); err != nil {
				logrus.Errorf("Unable to delete Stripe invoiceitem %s, err: %v", strItem.ID, err)
			}
		}
		if strSub != nil {
			logrus.Errorf("Subscribe failed, deleting Stripe subscription %s", strSub.ID)
			if err := stripesub.Cancel(strSub.ID, &stripe.SubParams{Customer: bp.StripeID.String}); err != nil {
				logrus.Errorf("Unable to delete Stripe subscription %s, err: %v", strSub.ID, err)
			}
		}

		if pcUse != nil {
			logrus.Errorf("Subscribe failed, deleting promo code claim %d", pcUse.ID)
			if err := pcUse.Delete(d); err != nil {
				logrus.Errorf("Unable to delete promo code claim %d, err: %v", pcUse.ID, err)
			}
		}

		return
	}

	if pcUse != nil {
		pcUse.SubscriptionID = sql.NullInt64{sub.ID, true}
		if err := pcUse.UpdateSubscriptionId(d); err != nil {
			logrus.Errorf("Unable to update promo code claim %d, err: %v", pcUse.ID, err)
		}
	}

	return
}

func (s *SubscriptionProvider) Cancel(d *db.Database, sub *subscription.Subscription) error {
	logrus.Infof("Subscription %d has been requested to be cancelled", sub.ID)

	bp, err := billingprofile.FindByID(d, sub.BillingProfileID)
	if err != nil {
		return err
	}

	pl, err := plan.FindByID(d, sub.PlanID)
	if err != nil {
		return err
	}

	// Set EndCancel to false to terminate subscription immediately if
	// subscription is terminated during free trial or is a free plan.
	endCancel := !(pl.IsFree() || sub.WithinFreeTrial())

	return stripesub.Cancel(
		sub.StripeID.String,
		&stripe.SubParams{Customer: bp.StripeID.String, EndCancel: endCancel},
	)
}

func (s *SubscriptionProvider) Upgrade(d *db.Database, sub *subscription.Subscription, p *plan.Plan, u *user.User) error {
	bp, err := billingprofile.FindByID(d, sub.BillingProfileID)
	if err != nil {
		return err
	}
	if !bp.EligibleForSubscription() {
		return ErrInvalidBillingState
	}

	existingPlan, err := plan.FindByID(d, sub.PlanID)
	if err != nil {
		return err
	}

	if !existingPlan.CanUpgradeToPlan(p) {
		return ErrCannotUpgrade
	}

	planID := p.StripePlanID.String
	if bp.State == billingprofile.StateExempt {
		planID = planID + "-exempt"
	}

	params := &stripe.SubParams{
		Customer: bp.StripeID.String,
		Plan:     planID,
	}
	compHostingMode := shouldApplyHostingModeCoupon(sub, existingPlan, p, bp)
	if compHostingMode {
		params.Coupon = compHostingModeCouponSlug
	}

	stripeSub, err := stripesub.Update(sub.StripeID.String, params)
	if err != nil {
		logrus.Errorf("Failed to upgrade Stripe subscription for subscription ID %d to plan %s: %v", sub.ID, p.Slug, err)
		return err
	}

	// if we're comping, we avoid immediately charging the user for any changes
	// as this will be an unnecessary charge (their plan price should not
	// change)
	if !compHostingMode {
		// create the new invoice to apply the prorata amount
		i, err := stripeinvoice.New(&stripe.InvoiceParams{Customer: bp.StripeID.String})
		if err != nil {
			// instead of performing another web request we just try to pay any oustanding invoices
			// and ignore if there was nothing to pay (exempt user, Stripe credit)
			if strings.Contains(err.Error(), "Nothing to invoice for customer") {
				i = nil
				logrus.Warnf("No need to create Stripe invoice for subscription ID %d to plan %s: %v", sub.ID, p.Slug, err)
			} else {
				logrus.Errorf("Failed to create Stripe invoice for subscription ID %d to plan %s: %v", sub.ID, p.Slug, err)
				return decodeStripeError(err)
			}
		}

		// immediately request the invoice be paid
		if i != nil && i.Amount >= 100 {
			_, err = stripeinvoice.Pay(i.ID, nil)
			if err != nil {
				logrus.Errorf("Failed to pay Stripe invoice %s for subscription ID %d to plan %s: %v", i.ID, sub.ID, p.Slug, err)
				return decodeStripeError(err)
			}
		}
	}

	sub.State = string(stripeSub.Status)
	sub.PlanID = p.ID
	sub.CurrentPeriodStart = pq.NullTime{time.Unix(stripeSub.PeriodStart, 0), true}
	sub.CurrentPeriodEnd = pq.NullTime{time.Unix(stripeSub.PeriodEnd, 0), true}

	if err := sub.Update(d); err != nil {
		return err
	}

	if err := s.upgradeHosts(d, sub); err != nil {
		logrus.Errorf("Error upgrading hosts for subscription %d, err: %v", sub.ID, err)
	}

	return nil
}

func (s *SubscriptionProvider) Reactivate(d *db.Database, sub *subscription.Subscription) error {
	logrus.Infof("Cancelled Subscription %d has been requested to be re-activated", sub.ID)

	bp, err := billingprofile.FindByID(d, sub.BillingProfileID)
	if err != nil {
		return err
	}

	cancelledPlan, err := plan.FindByID(d, sub.PlanID)
	if err != nil {
		return err
	}

	planID := cancelledPlan.StripePlanID.String
	if bp.State == billingprofile.StateExempt {
		planID = planID + "-exempt"
	}

	// https://support.stripe.com/questions/how-can-i-resume-a-subscription-after-it-has-been-canceled
	stripeSub, err := stripesub.Update(
		sub.StripeID.String,
		&stripe.SubParams{
			Customer: bp.StripeID.String,
			Plan:     planID,
		},
	)
	if err != nil {
		logrus.Errorf("Unable to re-activate Stripe subscription %d, err: %v.", sub.ID, err)
		return err
	}

	if stripeSub.EndCancel {
		return fmt.Errorf("An error occured when re-activating Stripe subscription %d. Expected Stripe response cancel_at_period_end=false, got cancel_at_period_end=true.", sub.ID)
	}

	return nil
}

func (s *SubscriptionProvider) upgradeHosts(d *db.Database, sub *subscription.Subscription) error {
	hosts := []*host.Host{}
	err := d.Select(&hosts, `SELECT
		hosts.*, pt.provider, pt.config
		FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
		WHERE subscription_id = $1 AND terminated_at IS NULL;`, sub.ID)
	if err != nil {
		return err
	}

	if len(hosts) == 0 {
		return nil
	}

	newPlan, err := plan.FindByID(d, sub.PlanID)
	if err != nil {
		return err
	}

	for _, h := range hosts {
		currPT, err := providertemplate.FindByID(d, h.ProviderTemplateID)
		if err != nil {
			return err
		}

		newPT, err := currPT.RespecTemplate(d, newPlan)
		if err != nil {
			if err == sql.ErrNoRows {
				logrus.Errorf("upgradeHosts host=%d: provider template for region %d / plan %d / provider %s does not exist", h.ID, h.RegionID, newPlan.ID, h.Provider)
				// at this point we have charged the user for the upgrade, so
				// we log the error and upgrade the host with new plan while
				// skipping the respec.  This will bring the user closer to
				// what they paid for
			} else {
				logrus.Errorf("upgradeHosts host=%d: Error locating new provider template for region %d / plan %d / provider %s, err: %v", h.ID, h.RegionID, newPlan.ID, h.Provider, err)
				continue
			}
		}

		err = d.WithTransaction(func(tx *sqlx.Tx) error {
			h.PlanID = newPlan.ID
			h.Size = sql.NullString{Valid: true, String: newPlan.Slug}
			h.Free = newPlan.IsFree()
			h.AutoShutdown = newPlan.AutoShutdown
			h.MaxContainers = sql.NullInt64{Valid: true, Int64: newPlan.MaxContainers}

			nstmt, err := tx.PrepareNamed(`UPDATE hosts
				SET (plan_id, size, free, max_containers, auto_shutdown, upgraded_at, updated_at)
				= (:plan_id, :size, :free, :max_containers, :auto_shutdown, now(), now())
				WHERE id = :id
				RETURNING *;`)
			if err != nil {
				return err
			}
			if err := nstmt.QueryRowx(h).StructScan(h); err != nil {
				return err
			}

			if newPT != nil {
				rs := &respec.Respec{
					HostID:             h.ID,
					ProviderTemplateID: newPT.ID,
					State:              respec.StatePending,
				}
				return rs.Insert(tx)
			}
			return nil
		})

		if err != nil {
			logrus.Errorf("upgradeHosts host=%d: Error completing plan change, err: %v", h.ID, err)
			continue
		}
	}

	return nil
}

func canSubscribe(u *user.User, o *org.Org, p *plan.Plan) error {
	// require an org is present for edu/org plans
	if (p.Org || p.Edu) && o == nil {
		return ErrOrgPlanProvided
	}

	// no further checks needed for a user
	if o == nil {
		return nil
	}

	// orgs must subscribe to an org plan
	if !p.Org {
		return ErrPlanNotOrg
	}

	// edu orgs must subscribe to edu plans
	if p.Edu != o.Edu {
		return ErrPlanNotEdu
	}

	return nil
}

func creditStripeInvoice(tx *sqlx.Tx, bp *billingprofile.BillingProfile, amount, subID int64, stripePlanID string, inv *invoice.Invoice) (*stripe.InvoiceItem, error) {
	if amount <= 0 {
		return nil, nil
	}

	if bp.State == billingprofile.StateFree {
		return nil, ErrPlanNotFree
	}

	invID := sql.NullInt64{Valid: false}
	if inv != nil {
		invID = sql.NullInt64{inv.ID, true}
	}
	cr, err := updateAvailableCredits(tx, bp.UserID, invID, sql.NullInt64{subID, true}, int64(amount))
	if err != nil {
		return nil, err
	}
	if cr != nil {
		strItem, err := stripeinvoiceitem.New(&stripe.InvoiceItemParams{
			Customer: bp.StripeID.String,
			Amount:   cr.Consumed,
			Currency: "usd",
			Desc:     fmt.Sprintf("Credit of $%.2f", float64(cr.Consumed*-1)/100.0),
		})
		return strItem, err
	}

	return nil, nil
}

func updateAvailableCredits(tx *sqlx.Tx, userID int64, invID sql.NullInt64, subID sql.NullInt64, amount int64) (*updatedAvailableCredits, error) {
	// insert into credits ONLY if the number of available of credits is greater than 0
	// TODO: use FOR UPDATE to ensure the rows are locked from other queries modifying
	availableCredits, err := credit.AvailableCredits(tx, userID)
	if err != nil {
		return nil, err
	}
	if availableCredits == 0 {
		return nil, nil
	}
	if availableCredits < amount {
		amount = availableCredits
	}
	deduction := amount * -1
	_, err = tx.Exec(`INSERT INTO credits(user_id, invoice_id, subscription_id, amount) VALUES ($1, $2, $3, $4)`, userID, invID, subID, deduction)
	if err != nil {
		return nil, err
	}
	return &updatedAvailableCredits{
		Consumed:  deduction,
		Available: availableCredits,
	}, nil
}

func decodeStripeError(err error) error {
	if sErr, ok := err.(*stripe.Error); ok {
		if sErr.Code == stripe.ExpiredCard {
			return ErrCardExpired
		} else if sErr.Code == stripe.CardDeclined {
			return ErrCardDeclined
		}
	}

	return err
}

func shouldApplyHostingModeCoupon(sub *subscription.Subscription, oldPlan, newPlan *plan.Plan, bp *billingprofile.BillingProfile) bool {
	if !oldPlan.HostingModePlanID.Valid || oldPlan.HostingModePlanID.Int64 != newPlan.ID {
		return false
	}
	if !bp.CompHostingModeBefore.Valid {
		return false
	}
	if sub.CreatedAt == nil || sub.CreatedAt.After(bp.CompHostingModeBefore.Time) {
		return false
	}
	return true
}
