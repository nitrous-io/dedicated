package subscriptions

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	stripe "github.com/stripe/stripe-go"
	stripeinvoice "github.com/stripe/stripe-go/invoice"
	stripeinvoiceitem "github.com/stripe/stripe-go/invoiceitem"
	stripesub "github.com/stripe/stripe-go/sub"
)

type Provider interface {
	Subscribe(d *db.Database, h *host.Host, p *plan.Plan, u *user.User) (*subscription.Subscription, error)
	Cancel(d *db.Database, sub *subscription.Subscription) error
	Upgrade(d *db.Database, sub *subscription.Subscription, p *plan.Plan, u *user.User) error

	HandleSubscriptionUpdated(d *db.Database, subID string) (*subscription.Subscription, error)
	HandleSubscriptionDeleted(d *db.Database, subID string) (*subscription.Subscription, error)
	HandleSubscriptionInvoiceCreated(d *db.Database, invID string) (*invoice.Invoice, error)
	HandleSubscriptionInvoiceFailed(d *db.Database, invID string) (*invoice.Invoice, error)
}

type hostStopFunc func(db *db.Database, h *host.Host, headers http.Header, ev hostcontrol.EventFunc, initiatorID int64) error

type SubscriptionProvider struct {
	HostStopFunc hostStopFunc
	Mailer       mailer.Mailer
}

var (
	ErrNotSubscribablePlan   = errors.New("Plan does not support subscriptions")
	ErrInvalidBillingState   = errors.New("Invalid billing state")
	ErrPlanNotFree           = errors.New("Plan not free but billing state in Free")
	ErrInvoiceAlreadyCreated = errors.New("Invoice already exists")

	ErrInvoiceLackingSubscription = errors.New("Invoice not attached to Subscription")
	ErrInvoiceAlreadyPaid         = errors.New("Invoice has already been paid")

	ErrCardExpired  = errors.New("Your Card has expired. Please try again with a recent card.")
	ErrCardDeclined = errors.New("Your Card was declined. Try contacting the bank directly for assistance.")
)

type updatedAvailableCredits struct {
	Consumed  int64
	Available int64
}

func NewProvider() *SubscriptionProvider {
	s := &SubscriptionProvider{}
	s.HostStopFunc = hostcontrol.Stop
	s.Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)
	return s
}

func (s *SubscriptionProvider) Subscribe(d *db.Database, h *host.Host, p *plan.Plan, u *user.User) (sub *subscription.Subscription, err error) {
	if !p.IsSubscription() {
		err = ErrNotSubscribablePlan
		return
	}

	bp, err := u.BillingProfile(d)
	if err != nil {
		return
	}

	if !bp.StripeID.Valid {
		bp, err = u.ActivateBillingProfile(d, "")
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
		sub = &subscription.Subscription{
			PlanID:           p.ID,
			BillingProfileID: bp.ID,
			State:            subscription.StateCreating,
		}
		err = sub.Insert(tx)
		if err != nil {
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

		strSub, err = stripesub.New(&stripe.SubParams{
			Customer: bp.StripeID.String,
			Plan:     planID,
		})
		if err != nil {
			logrus.Errorf("Failed to create Stripe subscription: %+v, err: %+v", strSub, err)
			strSub = nil

			if sErr, ok := err.(*stripe.Error); ok {
				if sErr.Code == stripe.ExpiredCard {
					return ErrCardExpired
				} else if sErr.Code == stripe.CardDeclined {
					return ErrCardDeclined
				}
			}

			return err
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
		err = nstmt.QueryRow(sub).StructScan(sub)
		if err != nil {
			return err
		}

		return h.UpdateColumn(tx, "subscription_id", sql.NullInt64{sub.ID, true})
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
	bp, err := u.BillingProfile(d)
	if err != nil {
		return err
	}
	if !bp.EligibleForSubscription() {
		return ErrInvalidBillingState
	}

	stripeSub, err := stripesub.Update(sub.StripeID.String, &stripe.SubParams{
		Customer: bp.StripeID.String,
		Plan:     p.StripePlanID.String,
	})
	if err != nil {
		logrus.Errorf("Failed to upgrade Stripe subscription for subscription ID %d to plan %s: %v", sub.ID, p.Slug, err)
		return err
	}

	sub.State = string(stripeSub.Status)
	sub.PlanID = p.ID
	sub.CurrentPeriodStart = pq.NullTime{time.Unix(stripeSub.PeriodStart, 0), true}
	sub.CurrentPeriodEnd = pq.NullTime{time.Unix(stripeSub.PeriodEnd, 0), true}

	return sub.Update(d)
}

func (s *SubscriptionProvider) HandleSubscriptionUpdated(d *db.Database, subID string) (*subscription.Subscription, error) {
	var sub *subscription.Subscription
	err := d.WithTransaction(func(tx *sqlx.Tx) error {
		var err error
		sub, err = findForUpdateByStripeID(tx, subID)
		if err != nil {
			return err
		}

		bp, err := billingprofile.FindByID(tx, sub.BillingProfileID)
		if err != nil {
			return err
		}

		strSub, err := stripesub.Get(subID, &stripe.SubParams{Customer: bp.StripeID.String})
		if err != nil {
			return err
		}

		sub.State = string(strSub.Status)
		if strSub.PeriodStart > 0 {
			sub.CurrentPeriodStart = pq.NullTime{time.Unix(strSub.PeriodStart, 0), true}
		} else {
			sub.CurrentPeriodStart = pq.NullTime{Valid: false}
		}
		if strSub.PeriodEnd > 0 {
			sub.CurrentPeriodEnd = pq.NullTime{time.Unix(strSub.PeriodEnd, 0), true}
		} else {
			sub.CurrentPeriodEnd = pq.NullTime{Valid: false}
		}
		sub.CancelPending = strSub.Canceled > 0

		nstmt, err := tx.PrepareNamed(`UPDATE subscriptions
			SET state = :state, current_period_start = :current_period_start,
			current_period_end = :current_period_end,
			cancel_pending = :cancel_pending, updated_at = now()
			WHERE id = :id
			RETURNING *;`)
		if err != nil {
			return err
		}
		return nstmt.QueryRowx(sub).StructScan(sub)
	})

	if err != nil {
		return nil, err
	}

	return sub, nil
}

func (s *SubscriptionProvider) HandleSubscriptionDeleted(d *db.Database, subID string) (*subscription.Subscription, error) {
	var sub *subscription.Subscription
	err := d.WithTransaction(func(tx *sqlx.Tx) error {
		var err error
		sub, err = findForUpdateByStripeID(tx, subID)
		if err != nil {
			return err
		}

		sub.State = subscription.StateDeleted
		sub.CancelledAt = pq.NullTime{time.Now(), true}
		sub.CancelPending = false

		nstmt, err := tx.PrepareNamed(`UPDATE subscriptions
			SET state = :state, cancelled_at = :cancelled_at, cancel_pending = :cancel_pending, updated_at = now() WHERE id = :id
			RETURNING *;`)
		if err != nil {
			return err
		}
		return nstmt.QueryRowx(sub).StructScan(sub)
	})

	if err != nil {
		return nil, err
	}

	if err := s.stopHosts(d, sub); err != nil {
		return nil, err
	}

	return sub, nil
}

func (s *SubscriptionProvider) HandleSubscriptionInvoiceCreated(d *db.Database, invID string) (*invoice.Invoice, error) {
	existingInv, err := invoice.FindByStripeID(d, invID)
	if err != sql.ErrNoRows {
		if err == nil && !existingInv.SubscriptionID.Valid {
			logrus.Warnf("HandleSubscriptionInvoiceCreated on invoice without subscription: %s", invID)
			return existingInv, nil
		}
		return nil, ErrInvoiceAlreadyCreated
	}

	sInv, err := stripeinvoice.Get(invID, nil)
	if err != nil {
		return nil, err
	}

	var inv *invoice.Invoice
	err = d.WithTransaction(func(tx *sqlx.Tx) error {
		sub, err := findForUpdateByStripeID(tx, sInv.Sub)
		if err != nil {
			return err
		}
		bp, err := billingprofile.FindByID(tx, sub.BillingProfileID)
		if err != nil {
			return err
		}

		pStart := time.Unix(sInv.Start, 0)
		pEnd := time.Unix(sInv.End, 0)
		inv = &invoice.Invoice{
			SubscriptionID: sql.NullInt64{sub.ID, true},
			TotalCharge:    sInv.Total,
			PeriodStart:    &pStart,
			PeriodEnd:      &pEnd,
			Attempts:       int(sInv.Attempts),
			StripeID:       sql.NullString{sInv.ID, true},
			UserID:         bp.UserID,
		}
		if sInv.Paid {
			if bp.IsFreeTier() || bp.IsExempt() {
				inv.State = invoice.StateNotPayable
			} else {
				inv.State = invoice.StatePaid
			}
		} else {
			inv.State = invoice.StateUnpaid
		}

		if err := inv.Insert(tx); err != nil {
			return err
		}

		if invoiceLines := sInv.Lines; invoiceLines != nil {
			for _, invItem := range invoiceLines.Values {
				desc := invItem.Desc
				if desc == "" {
					if invItem.Plan != nil {
						desc = fmt.Sprintf("Subscription to %s ($%.2f/%s)", invItem.Plan.Name, float64(invItem.Plan.Amount)/100.0, string(invItem.Plan.Interval))
					} else {
						desc = "Subscription"
					}
				}
				stripeID := invItem.ID
				if invItem.Type == stripeinvoice.TypeSubscription {
					stripeID = fmt.Sprintf("%s-%s", sInv.ID, invItem.ID)
				}
				item := invoiceitem.InvoiceItem{
					InvoiceID:         inv.ID,
					ItemType:          invoiceitem.ItemTypeSubscription,
					StripeID:          sql.NullString{stripeID, true},
					TotalStripeCharge: sql.NullInt64{invItem.Amount, true},
					Description:       sql.NullString{desc, true},
				}
				if item.TotalStripeCharge.Int64 < 0 {
					item.ItemType = invoiceitem.ItemTypeCredit
					item.Credit = sql.NullInt64{item.TotalStripeCharge.Int64 * -1, true}
				}
				if err := item.Insert(tx); err != nil {
					logrus.Errorf("Unable to create invoice item id %d, invoice item: %+v, err: %+v", inv.ID, invItem, err)
					return err
				}
			}
		}

		// Stripe tells us before charging a subscription by creating an invoice
		// this gives us an opportunity to credit the user before the charges go through
		// this should not abort the transaction but log a major error
		if !sInv.Paid {
			var stripePlanID string
			err = d.Get(&stripePlanID, "SELECT stripe_plan_id FROM plans WHERE id = $1", sub.PlanID)
			if err != nil {
				return err
			}
			crItem, err := creditStripeInvoice(tx, bp, inv.TotalCharge, sub.ID, stripePlanID, inv)
			if err != nil {
				return err
			}
			if crItem != nil {
				item := invoiceitem.InvoiceItem{
					InvoiceID:         inv.ID,
					ItemType:          invoiceitem.ItemTypeCredit,
					StripeID:          sql.NullString{crItem.ID, true},
					TotalStripeCharge: sql.NullInt64{crItem.Amount, true},
					Credit:            sql.NullInt64{crItem.Amount * -1, true},
				}
				if err := item.Insert(tx); err != nil {
					if err := stripeinvoiceitem.Del(crItem.ID); err != nil {
						logrus.Errorf("Unable to delete Stripe invoiceitem %s, err: %v", crItem.ID, err)
					}
					return err
				} else {
					// update the invoice total
					inv.TotalCharge += crItem.Amount
					if err := inv.Update(tx); err != nil {
						return err
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return inv, nil
}

func (s *SubscriptionProvider) HandleSubscriptionInvoiceFailed(d *db.Database, invID string) (*invoice.Invoice, error) {
	var (
		sub  *subscription.Subscription
		inv  *invoice.Invoice
		bp   *billingprofile.BillingProfile
		sInv *stripe.Invoice

		final bool
	)
	err := d.WithTransaction(func(tx *sqlx.Tx) error {
		inv = &invoice.Invoice{}
		err := tx.Get(inv, "SELECT * FROM invoices WHERE stripe_id = $1 FOR UPDATE", invID)
		if err != nil {
			return err
		}

		if inv.State == invoice.StatePaid {
			return fmt.Errorf("Invoice %d has already been paid", inv.ID)
		}

		sInv, err = stripeinvoice.Get(invID, nil)
		if err != nil {
			return err
		}

		if sInv.Paid {
			return fmt.Errorf("Stripe invoice %s has already been paid", sInv.ID)
		}

		// as part of a transition state we are going to allow non subscription
		// invoices through this method
		bp = &billingprofile.BillingProfile{}
		if inv.SubscriptionID.Valid {
			sub, err = subscription.Find(d, inv.SubscriptionID.Int64)
			if err != nil {
				return err
			}

			err = tx.Get(bp, "SELECT * FROM billing_profiles WHERE id = $1 FOR UPDATE", sub.BillingProfileID)
			if err != nil {
				return err
			}
		} else {
			err = tx.Get(bp, "SELECT * FROM billing_profiles WHERE user_id = $1 FOR UPDATE", inv.UserID)
			if err != nil {
				return err
			}
		}

		if bp == nil {
			return fmt.Errorf("Unable to locate BillingProfile for invoice %d", inv.ID)
		}

		if bp.State == billingprofile.StateExempt {
			return fmt.Errorf("Billing profile %d should not be failing in exempt state", bp.ID)
		}

		// update state of BillingProfile
		final = sInv.NextAttempt == 0
		if final {
			bp.State = billingprofile.StateDelinquent
		} else {
			bp.State = billingprofile.StatePaymentDue
		}

		nstmt, err := tx.PrepareNamed(`UPDATE billing_profiles
			SET state = :state, updated_at = now()
			WHERE id = :id
			RETURNING *;`)
		if err != nil {
			return err
		}

		if err := nstmt.QueryRowx(bp).StructScan(bp); err != nil {
			return err
		}

		// update invoice
		logrus.Warnf("Failed invoice %d: Next attempt at %d", inv.ID, sInv.NextAttempt)
		inv.State = invoice.StateUnpaid
		if final {
			inv.State = invoice.StateFailed
		}
		t := time.Now()
		if inv.FirstAttemptAt == nil || inv.FirstAttemptAt.IsZero() {
			inv.FirstAttemptAt = &t
		}
		inv.LastAttemptAt = &t
		inv.Attempts = int(sInv.Attempts)
		nstmt, err = tx.PrepareNamed(`UPDATE invoices
			SET state = :state, first_attempt_at = :first_attempt_at, last_attempt_at = :last_attempt_at,
			attempts = :attempts, updated_at = now()
			WHERE id = :id
			RETURNING *;`)
		if err != nil {
			return err
		}

		return nstmt.QueryRowx(inv).StructScan(inv)
	})

	if err != nil {
		return nil, err
	}

	if sub != nil && (sInv.Attempts > 1 || sInv.Closed) {
		if err := s.stopHosts(d, sub); err != nil {
			return nil, err
		}
	}

	if err := s.sendFailureEmail(d, int64(sInv.Attempts), final, inv, bp); err != nil {
		return nil, err
	}

	return inv, nil
}

func (s *SubscriptionProvider) stopHosts(d *db.Database, sub *subscription.Subscription) error {
	hosts := []*host.Host{}
	err := d.Select(&hosts, `SELECT
		hosts.*, pt.provider, pt.config
		FROM hosts
		INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
		INNER JOIN subscriptions ON subscriptions.id = hosts.subscription_id WHERE subscriptions.id = $1 AND terminated_at IS NULL;`, sub.ID)
	if err != nil {
		return err
	}

	headers := http.Header{}
	headers.Set(constants.NitrousUUIDHeader, fmt.Sprintf("subscription_terminated_%d", sub.ID))
	for _, h := range hosts {
		var initiatorID int64
		err = d.QueryRow(`SELECT id FROM users WHERE username = $1`, envvars.ProvisionerUser).Scan(&initiatorID)
		if err != nil {
			return err
		}

		if h.State == host.StateStopped {
			logrus.Infof("Host %d for subscription %d is already stopped", h.ID, sub.ID)
			continue
		}

		if err := s.HostStopFunc(d, h, headers, nil, initiatorID); err != nil {
			logrus.Errorf("Unable to stop host %d for subscription %d, err: %v", h.ID, sub.ID, err)
		}
	}

	return nil
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

func findForUpdateByStripeID(tx *sqlx.Tx, id string) (*subscription.Subscription, error) {
	sub := &subscription.Subscription{}
	err := tx.Get(sub, "SELECT * FROM subscriptions WHERE stripe_id = $1 FOR UPDATE", id)
	if err != nil {
		return nil, err
	}
	return sub, nil
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
