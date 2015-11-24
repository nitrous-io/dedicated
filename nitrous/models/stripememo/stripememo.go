package stripememo

import (
	"encoding/json"
	"fmt"
	"time"

	stripe "github.com/stripe/stripe-go"
	stripeinv "github.com/stripe/stripe-go/invoice"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
)

const (
	// default limit is 10. 24 gives us two years of invoices
	invoicesToFetch = "24"
)

var (
	ErrInvoiceFetchingProgress = fmt.Errorf("fetching invoices is already in progress")
)

type Invoice struct { // corresponds to stripe_invoices table
	BillingProfileID int64     `db:"billing_profile_id"`
	InvoiceJSON      string    `db:"invoice_json"`
	CreatedAt        time.Time `db:"created_at"`

	// this package's finders are to unmarshal InvoiceJSON and populate this
	*stripe.Invoice
}

func memoInvoices(bp *billingprofile.BillingProfile) ([]*Invoice, error) {
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
	case inv.Invoice == nil && inv.InvoiceJSON != "":
		return json.Unmarshal([]byte(inv.InvoiceJSON), &inv.Invoice)
	}
	return nil
}
