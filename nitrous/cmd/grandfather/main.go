package main

// grandfather command
// * used to create subscriptions for existing users during the switch to fixed pricing on Jul 1

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	stripe "github.com/stripe/stripe-go"
	stripeinvoiceitem "github.com/stripe/stripe-go/invoiceitem"
	stripesub "github.com/stripe/stripe-go/sub"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credit"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
)

var (
	dbConn *db.Database
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

// this is the query to determine which hosts are set to prepaid plans
// We would have to run UPDATEs individually for each plan to set to the correct one
// Billing profile state "exempt" would be a special case to be handled
// manually, since it is used for: contract users (flatiron), banned users (so we don't charge stolen cards), and admins
var hostsToUpgrade = `
	SELECT hosts.slug FROM hosts
	JOIN plans ON hosts.plan_id = plans.id
	JOIN users ON hosts.user_id = users.id
	JOIN billing_profiles bps on bps.user_id = users.id
	WHERE hosts.terminated_at IS NULL
		AND plans.type = 'usage-based'
		AND bps.state IN ('paid', 'payment_due', 'free')
`

func hostsToSubscribe(d db.QueryReceiver) ([]*host.Host, error) {
	hosts := []*host.Host{}
	// this query assumes that all the hosts for migration have already been
	// swapped to plans with 'prepaid' type
	err := d.Select(&hosts, `
	SELECT hosts.*
		FROM hosts
		JOIN plans ON hosts.plan_id = plans.id
		WHERE plans.type = 'prepaid'
			AND hosts.subscription_id IS NULL
			AND hosts.terminated_at IS NULL
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

var dryrun = true

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--live-run" {
		dryrun = false
	}

	envvars.CheckStripe()
	stripe.Key = envvars.StripeSecretKey

	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatal(envvars.DBEnvVar, "is not defined")
	}

	var err error
	dbConn, err = db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("healthcheck: db error: %s", err)
	}

	hosts, err := hostsToSubscribe(dbConn)
	if err != nil {
		logrus.Fatalf("hostsToSubscribe: %s", err)
	}

	for _, h := range hosts {
		// inefficient finds but this is one-off
		u, err := user.FindByID(dbConn, h.UserID)
		if err != nil {
			logrus.Fatalf("host loop user find: host slug %s, err %s", h.Slug, err)
		}
		pl, err := plan.FindByID(dbConn, h.PlanID)
		if err != nil {
			logrus.Fatalf("host loop plan find: host slug %s, err %s", h.Slug, err)
		}
		if dryrun {
			logrus.Infof("[DRY] will create subscription for host (id, slug, plan): (%d, %s, %s)", h.ID, h.Slug, pl.Slug)
			continue
		}
		s, err := Subscribe(dbConn, h, pl, u)
		if err != nil {
			logrus.Fatalf("host loop subscribe: host slug %s, err %s", h.Slug, err)
		}
		logrus.Infof("Created subscription id %d for host %s", s.ID, h.Slug)
	}
}

// copied from nitrous/subcriptions/subscriptions.go; the only difference
// (apart from qualifying imports) is the inclusion of the trial period end (so
// that existing workspaces get July 2015 free)
func Subscribe(d *db.Database, h *host.Host, p *plan.Plan, u *user.User) (sub *subscription.Subscription, err error) {
	if !p.IsSubscription() {
		err = subscriptions.ErrNotSubscribablePlan
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
		err = subscriptions.ErrInvalidBillingState
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

		if !(bp.State == billingprofile.StateExempt || bp.State == billingprofile.StateFree) {
			strItem, err = creditStripeInvoice(tx, bp, p.Price, sub.ID, p.StripePlanID.String, nil)
			if err != nil {
				return err
			}
		}

		planID := p.StripePlanID.String
		if bp.State == billingprofile.StateExempt {
			planID = planID + "-exempt"
		}

		sParams := &stripe.SubParams{
			Customer: bp.StripeID.String,
			Plan:     planID,
		}

		if bp.State != billingprofile.StateFree {
			sParams.TrialEnd = 1438387199 // this is the result of `date -d '2015-07-31 23:59:59' -u +%s`
		}

		strSub, err = stripesub.New(sParams)
		if err != nil {
			strSub = nil
			logrus.Errorf("Stripe subscription: %+v, err: %+v", strSub, err)

			sErr, ok := err.(*stripe.Error)
			if !ok {
				return sErr
			}

			if sErr.Code == stripe.ExpiredCard {
				return subscriptions.ErrCardExpired
			} else if sErr.Code == stripe.CardDeclined {
				return subscriptions.ErrCardDeclined
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

// copied unchanged from nitrous/subscriptions/subscriptions.go, apart from qualifying import
func creditStripeInvoice(tx *sqlx.Tx, bp *billingprofile.BillingProfile, amount, subID int64, stripePlanID string, inv *invoice.Invoice) (*stripe.InvoiceItem, error) {
	if amount <= 0 {
		return nil, nil
	}

	if bp.State == billingprofile.StateFree {
		return nil, subscriptions.ErrPlanNotFree
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

type updatedAvailableCredits struct {
	Consumed  int64
	Available int64
}

// copied unchanged from nitrous/subscriptions/subscriptions.go, apart from qualifying import
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
