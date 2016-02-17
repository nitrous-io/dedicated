package subscriptions

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/helpscout"
	stripe "github.com/stripe/stripe-go"
	stripeinvoice "github.com/stripe/stripe-go/invoice"
	stripeinvoiceitem "github.com/stripe/stripe-go/invoiceitem"
	stripesub "github.com/stripe/stripe-go/sub"
)

var (
	HelpScout helpscout.HelpScout

	nitrousMailbox  helpscout.MailboxRef
	nitrousCustomer helpscout.Person
	nitrousUser     helpscout.Person
)

const HelpscoutProxyEmail = "support@nitrous.io"

func init() {
	HelpScout = &helpscout.Conversation{}

	nitrousMailbox = helpscout.MailboxRef{ID: int(envvars.HelpScoutMailboxID), Name: envvars.HelpScoutMailboxName}
	nitrousCustomer = helpscout.Person{Type: "customer", Email: HelpscoutProxyEmail}
	nitrousUser = helpscout.Person{Type: "user", ID: envvars.HelpScoutUserID}
}

const minTimeBeforeFinalAttempt = (5 * 24) * time.Hour

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
		if strSub.TrialEnd > 0 {
			sub.TrialEnd = pq.NullTime{time.Unix(strSub.TrialEnd, 0), true}
		} else {
			sub.TrialEnd = pq.NullTime{Valid: false}
		}
		sub.CancelPending = strSub.Canceled > 0

		nstmt, err := tx.PrepareNamed(`UPDATE subscriptions
			SET state = :state, current_period_start = :current_period_start,
			current_period_end = :current_period_end,
			trial_end = :trial_end,
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

	if err := s.stopOrTerminateHosts(d, sub); err != nil {
		return nil, err
	}

	return sub, nil
}

func (s *SubscriptionProvider) HandleChargeSucceeded(d *db.Database, e *stripe.Event) error {
	stripeID, err := getStripeID(e)
	if err != nil {
		return err
	}

	_, err = d.Exec(`UPDATE billing_profiles SET failed_attempts=0 WHERE stripe_id=$1`, stripeID)
	if err != nil {
		return err
	}

	return nil
}

func (s *SubscriptionProvider) HandleChargeFailed(d *db.Database, e *stripe.Event) error {
	stripeID, err := getStripeID(e)
	if err != nil {
		return err
	}

	return d.WithTransaction(func(tx *sqlx.Tx) error {
		var paidBefore bool
		if err := tx.QueryRow(`SELECT EXISTS(select 1 FROM invoices LEFT JOIN billing_profiles ON invoices.user_id=billing_profiles.user_id AND invoices.state='paid' where billing_profiles.stripe_id=$1 ORDER BY invoices.created_at DESC);`, stripeID).Scan(&paidBefore); err != nil {
			return err
		}
		if paidBefore {
			return nil
		}

		bp := &billingprofile.BillingProfile{}
		if err := tx.QueryRowx(`UPDATE billing_profiles SET failed_attempts=failed_attempts+1 WHERE stripe_id=$1 RETURNING user_id, failed_attempts;`, stripeID).StructScan(bp); err != nil {
			return err
		}
		if !bp.MaxAttemptsReached() {
			return nil
		}

		u := &user.User{}
		if err := tx.QueryRowx(`UPDATE users SET suspicious=CONCAT_WS(',', suspicious, 'failed_charge') WHERE id=$1 RETURNING id, username, email;`, bp.UserID).StructScan(u); err != nil {
			return err
		}

		go func() {
			if err := SendChargeFailedTicket(u, e); err != nil {
				logrus.Errorf("Failed to send charge failed ticket: %v", err)
			}
		}()

		return nil
	})
}

func getStripeID(e *stripe.Event) (string, error) {
	source := e.Data.Obj["source"]
	if source == nil {
		return "", fmt.Errorf("Stripe failed to provide source: %+v", e.Data.Obj)
	}

	stripeID := e.GetObjValue("source", "customer")
	if stripeID == "" {
		return "", fmt.Errorf("Stripe failed to provide customer: %+v", e.Data.Obj)
	}
	return stripeID, nil
}

func SendChargeFailedTicket(u *user.User, e *stripe.Event) error {
	host := ""
	switch envvars.HostHostname {
	case "n2odev.com":
		host = "[DEV] "
	case "n2ostaging.com":
		host = "[STAGING] "
	}

	subject := fmt.Sprintf("%s[ALERT] Stripe - Multiple Failed Charge Attempts", host)
	body := fmt.Sprintf("<strong>User</strong>: ID: %v, Username: %s, Email: %s", u.ID, u.Username, u.Email)
	nitrousMessage := helpscout.NewThread("message", nitrousUser, body)
	threads := []*helpscout.Thread{nitrousMessage}

	if err := HelpScout.CreateConversation(nitrousCustomer, subject, nitrousMailbox, threads); err != nil {
		return fmt.Errorf("author: %+v, subject: %s, mailbox: %+v, threads: %+v. err: %v", nitrousCustomer, subject, nitrousMailbox, threads[0], err)
	}
	return nil
}

func (s *SubscriptionProvider) HandleSubscriptionInvoiceCreated(d *db.Database, invID string) (*invoice.Invoice, error) {
	existingInv, err := invoice.FindByStripeID(d, invID)
	switch err {
	case nil:
		if !existingInv.SubscriptionID.Valid {
			logrus.Warnf("HandleSubscriptionInvoiceCreated on invoice without subscription: %#v", existingInv)
			return existingInv, nil
		}
		return nil, ErrInvoiceAlreadyCreated
	case sql.ErrNoRows:
		// No existing invoice - continue.
		logrus.Infof("HandleSubscriptionInvoiceCreated handling invoice created, Stripe ID %s", invID)
	default:
		logrus.Warnf("HandleSubscriptionInvoiceCreated error finding invoice by Stripe ID %s, err: %v", invID, err)
		return nil, err
	}

	sInv, err := stripeinvoice.Get(invID, nil)
	if err != nil {
		return nil, err
	}

	var inv *invoice.Invoice
	tx, err := d.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if sInv.Sub == "" {
		if containsProration(sInv) {
			logrus.Infof("HandleSubscriptionInvoiceCreated handling Stripe invoice with prorate items, will not commit to DB. Stripe invoice: %#v", sInv)
			return nil, nil
		}
	}

	sub, err := findForUpdateByStripeID(tx, sInv.Sub)
	if err != nil {
		return nil, err
	}
	bp, err := billingprofile.FindByID(tx, sub.BillingProfileID)
	if err != nil {
		return nil, err
	}

	// SPECIAL CASE: When we switched from usage-based plans to prepaid
	// subscriptions on 1 July 2015, some users had deferred invoice items
	// because they owed us < $1. When we grandfathered them into a
	// subscription plan, Stripe paid the subscription of $0 (because of
	// the 1-month free trial we gave them) AND any outstanding invoice
	// items (i.e. the deferred invoice items).
	// So yeah crap - now we have to handle this special case by:
	// 1. If the invoice items Stripe sent us has both a subscription and
	//    invoice items that already exist in the database, we check whether
	//    the associated invoice records for the invoice items have
	//    state="deferred".
	// 2. If all the invoices are deferred, we associate their invoice items to
	//    a newly created invoice.
	// 3. Then we delete the deferred invoices.
	// 4. If anything is unexpected, e.g. the invoice items don't belong
	//    to a deferred invoice, we log verbosely.
	containsSubscription := false
	invoiceItemIDs := []string{}
	if sInv.Lines != nil {
		for _, invItem := range sInv.Lines.Values {
			switch invItem.Type {
			case stripeinvoice.TypeInvoiceItem:
				if invItem.Amount < 0 {
					// Ignore credit items because they will not exist in the database.
					logrus.Infof("HandleSubscriptionInvoiceCreated ignoring credit invoice item: %#v", invItem)
					continue
				}
				invoiceItemIDs = append(invoiceItemIDs, invItem.ID)
			case stripeinvoice.TypeSubscription:
				containsSubscription = true
			}
		}
	}

	if containsSubscription && len(invoiceItemIDs) > 0 {
		logrus.Infof("HandleSubscriptionInvoiceCreated handling Stripe invoice with both subscription and invoiceitems, Stripe invoice: %#v", sInv)

		invItems := []*invoiceitem.InvoiceItem{}
		for _, stripeID := range invoiceItemIDs {
			item := &invoiceitem.InvoiceItem{}
			err := tx.QueryRowx("SELECT * FROM invoice_items WHERE stripe_id = $1;", stripeID).StructScan(item)
			if err != nil {
				if err == sql.ErrNoRows {
					logrus.Errorf("HandleSubscriptionInvoiceCreated got invoice lines with subscription but invoiceitem %q doesn't exist in database, Stripe invoice: %#v", stripeID, sInv)
				}
				return nil, err
			}
			invItems = append(invItems, item)
		}

		// All the invoice items should belong to a deferred invoice.
		invoices := []*invoice.Invoice{}
		for _, item := range invItems {
			inv, err := invoice.Find(tx, item.InvoiceID)
			if err != nil {
				return nil, err
			}
			if inv.State != invoice.StateDeferred {
				logrus.Errorf("HandleSubscriptionInvoiceCreated got invoice with invoiceitem %#v that belongs to an invoice that is not deferred, Stripe invoice: %#v", item, sInv)
				return nil, fmt.Errorf("unexpected data for Stripe invoice %s", sInv.ID)
			}
			invoices = append(invoices, inv)
		}

		if !sInv.Paid {
			return nil, fmt.Errorf("Unexpected unpaid Stripe invoice %s", sInv.ID)
		}

		// Create a new invoice.
		pStart := time.Unix(sInv.Start, 0)
		pEnd := time.Unix(sInv.End, 0)
		inv := &invoice.Invoice{
			SubscriptionID:   sql.NullInt64{sub.ID, true},
			TotalCharge:      sInv.Amount,
			PeriodStart:      &pStart,
			PeriodEnd:        &pEnd,
			Attempts:         int(sInv.Attempts),
			StripeID:         sql.NullString{sInv.ID, true},
			UserID:           bp.UserID,
			BillingProfileID: sql.NullInt64{bp.ID, true},
			State:            invoice.StatePaid,
		}
		if err := inv.Insert(tx); err != nil {
			return nil, err
		}

		// Point all the deferred invoice items to the new invoice.
		for _, item := range invItems {
			item.InvoiceID = inv.ID
			if err := item.Update(tx); err != nil {
				return nil, err
			}
		}

		// Delete the deferred invoice(s).
		for _, inv := range invoices {
			if err := inv.Delete(tx); err != nil {
				return nil, err
			}
		}

		// Create invoice item for just the subscription invoice item.
		for _, invItem := range sInv.Lines.Values {
			if invItem.Type != stripeinvoice.TypeSubscription {
				continue
			}

			desc := invItem.Desc
			// Set a helpful description if it's empty.
			if desc == "" {
				if invItem.Plan != nil {
					desc = fmt.Sprintf("Subscription to %s ($%.2f/%s)", invItem.Plan.Name, float64(invItem.Plan.Amount)/100.0, string(invItem.Plan.Interval))
				} else {
					desc = "Subscription"
				}
			}

			stripeID := fmt.Sprintf("%s-%s", sInv.ID, invItem.ID)
			item := &invoiceitem.InvoiceItem{
				InvoiceID:         inv.ID,
				ItemType:          invoiceitem.ItemTypeSubscription,
				StripeID:          sql.NullString{stripeID, true},
				TotalStripeCharge: sql.NullInt64{invItem.Amount, true},
				Description:       sql.NullString{desc, true},
			}
			if err := item.Insert(tx); err != nil {
				logrus.Errorf("Unable to create subscription invoice item: %+v, err: %v", invItem, err)
				return nil, err
			}
		}

		if err := tx.Commit(); err != nil {
			return nil, err
		}

		return inv, nil
	}
	// END of special case.

	pStart := time.Unix(sInv.Start, 0)
	pEnd := time.Unix(sInv.End, 0)
	inv = &invoice.Invoice{
		SubscriptionID:   sql.NullInt64{sub.ID, true},
		TotalCharge:      sInv.Amount,
		PeriodStart:      &pStart,
		PeriodEnd:        &pEnd,
		Attempts:         int(sInv.Attempts),
		StripeID:         sql.NullString{sInv.ID, true},
		UserID:           bp.UserID,
		BillingProfileID: sql.NullInt64{bp.ID, true},
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
		return nil, err
	}

	if sInv.Lines != nil {
		for _, invItem := range sInv.Lines.Values {
			desc := invItem.Desc
			// Set a helpful description if it's empty.
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
				return nil, err
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
			return nil, err
		}
		crItem, err := creditStripeInvoice(tx, bp, inv.TotalCharge, sub.ID, stripePlanID, inv)
		if err != nil {
			return nil, err
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
				return nil, err
			} else {
				// update the invoice total
				inv.TotalCharge += crItem.Amount
				if err := inv.Update(tx); err != nil {
					return nil, err
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
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
			err = tx.Get(bp, "SELECT * FROM billing_profiles WHERE user_id = $1 AND org_id IS NULL FOR UPDATE", inv.UserID)
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
		if final && time.Now().Sub(*inv.CreatedAt) <= minTimeBeforeFinalAttempt {
			return fmt.Errorf("Received an invoice failed event with next_attempt=0 for invoice ID %d which is < %v old", inv.ID, minTimeBeforeFinalAttempt)
		}
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
			// if trial_end and current_period_start are non-null and equal, we
			// know that
			// 1. this is a free trial
			// 2. the first invoice has failed
			// In this circumstance, we automatically cancel the subscription
			if sub != nil {
				if sub.TrialEnd.Valid && sub.CurrentPeriodStart.Valid && sub.TrialEnd.Time.Equal(sub.CurrentPeriodStart.Time) {
					if err := s.Cancel(d, sub); err != nil {
						// if this fails we will manually cancel it
						logrus.Warnf("Failed to cancel sub that failed free trial first invoice %s", sub.StripeID)
					}
				}
			}
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
		if err := s.stopOrTerminateHosts(d, sub); err != nil {
			return nil, err
		}
	}

	if err := s.sendFailureEmail(d, int64(sInv.Attempts), sInv.NextAttempt, final, inv, bp); err != nil {
		return nil, err
	}

	return inv, nil
}

func (s *SubscriptionProvider) stopOrTerminateHosts(d *db.Database, sub *subscription.Subscription) error {
	hosts := []*host.Host{}
	err := d.Select(&hosts, `SELECT
		hosts.*, pt.provider, pt.config
		FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
			INNER JOIN subscriptions ON subscriptions.id = hosts.subscription_id
		WHERE subscriptions.id = $1
		AND terminated_at IS NULL;`, sub.ID)
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

		if h.Free {
			if h.State == host.StateTerminated {
				logrus.Infof("Host %d for subscription %d is already terminated", h.ID, sub.ID)
				continue
			}

			err = d.WithTransaction(func(tx *sqlx.Tx) error {
				return s.HostTerminateFunc(tx, h, headers, initiatorID, h.State)
			})
			if err != nil {
				logrus.Errorf("Unable to terminate host %d for subscription %d, err: %v", h.ID, sub.ID, err)
			}
		} else {
			if h.State == host.StateStopped {
				logrus.Infof("Host %d for subscription %d is already stopped", h.ID, sub.ID)
				continue
			}
			if err := s.HostStopFunc(d, h, headers, initiatorID); err != nil {
				logrus.Errorf("Unable to stop host %d for subscription %d, err: %v", h.ID, sub.ID, err)
			}
		}
	}

	return nil
}

func findForUpdateByStripeID(tx *sqlx.Tx, id string) (*subscription.Subscription, error) {
	sub := &subscription.Subscription{}
	err := tx.Get(sub, "SELECT * FROM subscriptions WHERE stripe_id = $1 FOR UPDATE", id)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func containsProration(sInv *stripe.Invoice) bool {
	if sInv.Lines != nil {
		for _, invItem := range sInv.Lines.Values {
			if invItem.Proration {
				return true
			}
		}
	}

	return false
}
