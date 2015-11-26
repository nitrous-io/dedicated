package stripememo

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	stripe "github.com/stripe/stripe-go"
	stripeinv "github.com/stripe/stripe-go/invoice"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
)

const (
	// default limit is 10. 24 gives us two years of invoices (less if the user
	// upgraded)
	invoicesToFetch = "24"
)

var (
	ErrInvoiceFetchingProgress = fmt.Errorf("fetching invoices is already in progress")
)

type Invoice struct { // corresponds to stripe_invoices table
	BillingProfileID int64     `db:"billing_profile_id"`
	InvoiceJSON      string    `db:"invoice_json"`
	CreatedAt        time.Time `db:"created_at"`

	// this is extracted from the 'date' field of InvoiceJSON in order to be sortable
	InvoicedAt time.Time `db:"invoiced_at"`

	// this package's finders are to unmarshal InvoiceJSON and populate this
	*stripe.Invoice
}

// Invoices checks for memoized copies in the db (created within 24 hours),
// otherwise it populates the db with invoices for given billingprofile
func Invoices(d *db.Database, bp *billingprofile.BillingProfile) ([]*Invoice, error) {
	var memoInvs []*Invoice
	// we don't want repeated calls to the endpoint to fire multiple Stripe
	// calls if the db has not been populated yet, so we use a db lock
	err := d.WithTransaction(func(tx *sqlx.Tx) error {
		locked, err := db.TransactionLock(tx, db.INVOICE_USER_LOCK, int(bp.ID))
		if err != nil {
			return err
		} else if !locked {
			return ErrInvoiceFetchingProgress
		}
		hasRecent, err := hasRecentInvoices(d, bp.ID)
		if err != nil {
			return err
		}
		if hasRecent {
			memoInvs, err = dbRecentInvoices(tx, bp.ID)
			return err
		}

		// delete old invoices to avoid having to compare for repeats
		if _, err := tx.Exec(`DELETE FROM stripe_invoices
		WHERE billing_profile_id = $1`, bp.ID); err != nil {
			return err
		}
		fis, err := fetchedInvoices(bp)
		if err != nil {
			return err
		}
		for _, fi := range fis {
			if err := fi.Insert(d); err != nil {
				return err
			}
			memoInvs = append(memoInvs, fi)
		}
		return nil
	})
	return memoInvs, err
}

func dbRecentInvoices(d db.QueryReceiver, bpid int64) ([]*Invoice, error) {
	var invs []*Invoice
	err := d.Select(&invs, `SELECT * FROM stripe_invoices
	WHERE billing_profile_id = $1
	ORDER BY invoiced_at
	LIMIT $2
	`, bpid, invoicesToFetch)
	for _, inv := range invs {
		if err := inv.populate(); err != nil {
			return nil, err
		}
	}
	return invs, err
}

func hasRecentInvoices(d db.QueryReceiver, bpid int64) (bool, error) {
	var hasRecent bool
	err := d.Get(&hasRecent, `SELECT count(*) > 0 FROM stripe_invoices
		WHERE billing_profile_id = $1
		AND created_at > now() - interval '24 hours'
		`, bpid)
	return hasRecent, err
}

func fetchedInvoices(bp *billingprofile.BillingProfile) ([]*Invoice, error) {
	var memoInvs []*Invoice
	invs, err := stripeInvoices(bp.StripeID.String)
	if err != nil {
		return nil, err
	}
	for _, strinv := range invs {
		inv := &Invoice{
			BillingProfileID: bp.ID,
			Invoice:          strinv,
		}
		if err := inv.populate(); err != nil {
			return nil, err
		}
		memoInvs = append(memoInvs, inv)
	}
	return memoInvs, nil
}

// stripeInvoices returns invoices from Stripe, most recent first (guaranteed in Stripe API docs)
func stripeInvoices(custID string) ([]*stripe.Invoice, error) {
	var invs []*stripe.Invoice
	params := &stripe.InvoiceListParams{}
	params.Filters.AddFilter("limit", "", invoicesToFetch)
	params.Filters.AddFilter("customer", "", custID)
	invlist := stripeinv.List(params)
	if err := invlist.Err(); err != nil {
		return nil, err
	}
	for invlist.Next() {
		if inv := invlist.Invoice(); inv != nil {
			invs = append(invs, inv)
		}
	}
	return invs, nil
}

func (inv *Invoice) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO stripe_invoices
	(billing_profile_id, invoice_json, invoiced_at) VALUES
	(:billing_profile_id, :invoice_json, :invoiced_at)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(inv).StructScan(inv)
}

// populate will fill in either the Invoice or InvoiceJSON fields if only one
// is present.  If both are present, no-op.  If neither are present, error
func (inv *Invoice) populate() error {
	switch {
	case inv.Invoice == nil && inv.InvoiceJSON == "":
		return fmt.Errorf("invoice missing fields: %+v", inv)
	case inv.Invoice != nil && inv.InvoiceJSON != "":
		// both are non-null/zero, no-op
		return nil
	case inv.Invoice != nil && inv.InvoiceJSON == "":
		j, err := json.Marshal(inv.Invoice)
		if err != nil {
			return err
		}
		inv.InvoiceJSON = string(j)
		inv.InvoicedAt = time.Unix(inv.Date, 0)
	case inv.Invoice == nil && inv.InvoiceJSON != "":
		if err := json.Unmarshal([]byte(inv.InvoiceJSON), &inv.Invoice); err != nil {
			return err
		}
		inv.InvoicedAt = time.Unix(inv.Date, 0)
	}
	return nil
}
