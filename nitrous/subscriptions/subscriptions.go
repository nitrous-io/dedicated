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
	stripe "github.com/stripe/stripe-go"
	stripeinvoice "github.com/stripe/stripe-go/invoice"
	stripeinvoiceitem "github.com/stripe/stripe-go/invoiceitem"
	stripesub "github.com/stripe/stripe-go/sub"
)

type Provider interface {
	Subscribe(d *db.Database, h *host.Host, p *plan.Plan, u *user.User) (sub *subscription.Subscription, err error)
	Cancel(d *db.Database, sub *subscription.Subscription) (err error)

	HandleSubscriptionUpdated(d *db.Database, subID string) error
	HandleSubscriptionDeleted(d *db.Database, subID string) error
	HandleSubscriptionInvoiceCreated(d *db.Database, invID string) error
}

type hostStopFunc func(db *db.Database, h *host.Host, headers http.Header, ev hostcontrol.EventFunc, initiatorID int64) error

type SubscriptionProvider struct {
	HostStopFunc hostStopFunc
}

var (
	ErrNotSubscribablePlan   = errors.New("Plan does not support subscriptions")
	ErrInvalidBillingState   = errors.New("Invalid billing state")
	ErrPlanNotFree           = errors.New("Plan not free but billing state in Free")
	ErrInvoiceAlreadyCreated = errors.New("Invoice already exists")
)

type updatedAvailableCredits struct {
	Consumed  int64
	Available int64
}

func NewProvider() *SubscriptionProvider {
	s := &SubscriptionProvider{}
	s.HostStopFunc = hostcontrol.Stop
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
			strSub = nil
			logrus.Errorf("Stripe subscription: %+v", strSub)
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

func (s *SubscriptionProvider) Cancel(d *db.Database, sub *subscription.Subscription) (err error) {
	bp, err := billingprofile.FindByID(d, sub.BillingProfileID)
	if err != nil {
		return err
	}

	logrus.Infof("Subscription %d has been requested to be cancelled", sub.ID)

	return stripesub.Cancel(
		sub.StripeID.String,
		&stripe.SubParams{Customer: bp.StripeID.String, EndCancel: true},
	)
}

func (s *SubscriptionProvider) HandleSubscriptionUpdated(d *db.Database, subID string) error {
	return d.WithTransaction(func(tx *sqlx.Tx) error {
		sub, err := findForUpdateByStripeID(tx, subID)
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
    SET state = :state, current_period_start = :current_period_start, current_period_end = :current_period_end,
    cancel_pending = :cancel_pending, updated_at = now()
    WHERE id = :id
    RETURNING *;`)
		if err != nil {
			return err
		}
		return nstmt.QueryRowx(sub).StructScan(sub)
	})
}

func (s *SubscriptionProvider) HandleSubscriptionDeleted(d *db.Database, subID string) error {
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
		return err
	}

	hosts := []*host.Host{}
	err = d.Select(&hosts, `SELECT
		hosts.*, pt.provider, pt.config
		FROM hosts
		INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
		INNER JOIN subscriptions ON subscriptions.id = hosts.subscription_id WHERE subscriptions.id = $1`, sub.ID)
	if err != nil {
		return err
	}

	for _, host := range hosts {
		var initiatorID int64
		err = d.QueryRow(`SELECT id FROM users WHERE username = $1`, envvars.ProvisionerUser).Scan(&initiatorID)
		if err != nil {
			return err
		}

		headers := http.Header{}
		headers.Set(constants.NitrousUUIDHeader, "subscription_terminated")

		return s.HostStopFunc(d, host, headers, nil, initiatorID)
	}
	return nil
}

func (s *SubscriptionProvider) HandleSubscriptionInvoiceCreated(d *db.Database, invID string) error {
	_, err := invoice.FindByStripeID(d, invID)
	if err != sql.ErrNoRows {
		return ErrInvoiceAlreadyCreated
	}

	sInv, err := stripeinvoice.Get(invID, nil)
	if err != nil {
		return err
	}

	return d.WithTransaction(func(tx *sqlx.Tx) error {
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
		inv := &invoice.Invoice{
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
				item := invoiceitem.InvoiceItem{
					InvoiceID:         inv.ID,
					ItemType:          invoiceitem.ItemTypeSubscription,
					StripeID:          sql.NullString{invItem.ID, true},
					TotalStripeCharge: sql.NullInt64{invItem.Amount, true},
					Description:       sql.NullString{desc, true},
				}
				if item.TotalStripeCharge.Int64 < 0 {
					item.ItemType = invoiceitem.ItemTypeCredit
					item.Credit = sql.NullInt64{item.TotalStripeCharge.Int64 * -1, true}
				}
				if err := item.Insert(tx); err != nil {
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
