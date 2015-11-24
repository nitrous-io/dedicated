package stripememo

import (
	"time"

	stripe "github.com/stripe/stripe-go"
)

type Invoice struct { // corresponds to stripe_invoices table
	BillingProfileID int64     `db:"billing_profile_id"`
	InvoiceJSON      string    `db:"invoice_json"`
	CreatedAt        time.Time `db:"created_at"`

	// this package's finders are to unmarshal InvoiceJSON and populate this
	*stripe.Invoice
}
