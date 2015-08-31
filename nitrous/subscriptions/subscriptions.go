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
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
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

type Provider interface {
	Subscribe(d *db.Database, h *host.Host, p *plan.Plan, u *user.User, o *org.Org) (*subscription.Subscription, error)
	Cancel(d *db.Database, sub *subscription.Subscription) error
	Upgrade(d *db.Database, sub *subscription.Subscription, p *plan.Plan, u *user.User) error
	Reactivate(d *db.Database, sub *subscription.Subscription) error

	HandleSubscriptionUpdated(d *db.Database, subID string) (*subscription.Subscription, error)
	HandleSubscriptionDeleted(d *db.Database, subID string) (*subscription.Subscription, error)
	HandleSubscriptionInvoiceCreated(d *db.Database, invID string) (*invoice.Invoice, error)
	HandleSubscriptionInvoiceFailed(d *db.Database, invID string) (*invoice.Invoice, error)
}

type couponGetFunc func(string, *stripe.CouponParams) (*stripe.Coupon, error)

type hostStopFunc func(db *db.Database, h *host.Host, headers http.Header, ev hostcontrol.EventFunc, initiatorID int64) error

type SubscriptionProvider struct {
	HostStopFunc hostStopFunc
	Mailer       mailer.Mailer
}

var (
	couponGetFn couponGetFunc

	ErrNotSubscribablePlan   = errors.New("Plan does not support subscriptions")
	ErrInvalidBillingState   = errors.New("Invalid billing state")
	ErrPlanNotFree           = errors.New("Plan not free but billing state in Free")
	ErrInvoiceAlreadyCreated = errors.New("Invoice already exists")

	ErrInvoiceLackingSubscription = errors.New("Invoice not attached to Subscription")
	ErrInvoiceAlreadyPaid         = errors.New("Invoice has already been paid")

	ErrCardExpired  = errors.New("Your Card has expired. Please try again with a recent card.")
	ErrCardDeclined = errors.New("Your Card was declined. Try contacting the bank directly for assistance.")

	ErrCannotUpgrade = errors.New("You cannot upgrade to this plan")
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

func (s *SubscriptionProvider) Subscribe(d *db.Database, h *host.Host, p *plan.Plan, u *user.User, o *org.Org) (sub *subscription.Subscription, err error) {
	if !p.IsSubscription() {
		err = ErrNotSubscribablePlan
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
		nstmt, err := tx.PrepareNamed(`UPDATE subscriptions
			SET state = :state, stripe_id = :stripe_id,
			current_period_start = :current_period_start, current_period_end = :current_period_end, updated_at = now()
			WHERE id = :id
			RETURNING *;`)
		if err != nil {
			return err
		}
		if err := nstmt.QueryRow(sub).StructScan(sub); err != nil {
			return err
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
	}

	return
}

func (s *SubscriptionProvider) Cancel(d *db.Database, sub *subscription.Subscription) error {
	bp, err := billingprofile.FindByID(d, sub.BillingProfileID)
	if err != nil {
		return err
	}

	logrus.Infof("Subscription %d has been requested to be cancelled", sub.ID)

	pl, err := plan.FindByID(d, sub.PlanID)
	if err != nil {
		return err
	}

	endCancel := !pl.IsFree()

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

	stripeSub, err := stripesub.Update(sub.StripeID.String, &stripe.SubParams{
		Customer: bp.StripeID.String,
		Plan:     planID,
	})
	if err != nil {
		logrus.Errorf("Failed to upgrade Stripe subscription for subscription ID %d to plan %s: %v", sub.ID, p.Slug, err)
		return err
	}

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

		newPT, err := providertemplate.FindForRespec(d, &region.Region{ID: h.RegionID}, newPlan, h.Provider, currPT.Hotswap)
		if err != nil {
			if err == sql.ErrNoRows {
				logrus.Errorf("upgradeHosts host=%d: provider template for region %d / plan %d / provider %s does not exist", h.ID, h.RegionID, newPlan.ID, h.Provider)
				continue
			}
			logrus.Errorf("upgradeHosts host=%d: Error locating new provider template for region %d / plan %d / provider %s, err: %v", h.ID, h.RegionID, newPlan.ID, h.Provider, err)
			continue
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

			rs := &respec.Respec{
				HostID:             h.ID,
				ProviderTemplateID: newPT.ID,
				State:              respec.StatePending,
			}
			return rs.Insert(tx)
		})

		if err != nil {
			logrus.Errorf("upgradeHosts host=%d: Error completing plan change, err: %v", h.ID, err)
			continue
		}
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
