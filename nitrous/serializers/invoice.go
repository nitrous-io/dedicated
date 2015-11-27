package serializers

import (
	"time"

	stripe "github.com/stripe/stripe-go"

	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/stripememo"
)

type Invoice struct {
	ID           int64         `json:"id"`
	UserID       int64         `json:"user_id"`
	TotalCharge  int64         `json:"total_charge"`
	PeriodStart  *time.Time    `json:"period_start"`
	PeriodEnd    *time.Time    `json:"period_end"`
	State        string        `json:"state"`
	Attempted    bool          `json:"attempted"`
	Items        []InvoiceItem `json:"items"`
	Subscription bool          `json:"subscription"`

	TotalCredit     int64 `json:"total_credit"`
	AvailableCredit int64 `json:"available_credit"`

	StripeID string `json:"sid,omitempty"`
}

func SerializeInvoice(i *invoice.Invoice) *Invoice {
	if i == nil {
		return nil
	}
	inv := &Invoice{
		ID:           i.ID,
		UserID:       i.UserID,
		TotalCharge:  i.TotalCharge,
		PeriodStart:  i.PeriodStart,
		State:        i.State,
		PeriodEnd:    i.PeriodEnd,
		Attempted:    i.Attempts > 0,
		Subscription: i.SubscriptionID.Valid,
	}
	if i.AvailableCredit.Valid {
		inv.AvailableCredit = i.AvailableCredit.Int64
	}
	return inv
}

func SerializeStripeInvoice(mi *stripememo.Invoice) *Invoice {
	if mi == nil {
		return nil
	}
	si, err := mi.StripeInvoice()
	if err != nil {
		return nil
	}
	pstart, pend := time.Unix(si.Start, 0), time.Unix(si.End, 0)
	inv := &Invoice{
		StripeID:     si.ID,
		UserID:       mi.UserID,
		TotalCharge:  si.Total,
		State:        stateFromStripe(si),
		PeriodStart:  &pstart,
		PeriodEnd:    &pend,
		Attempted:    si.Attempted,
		Subscription: si.Sub != "",
	}
	var items []InvoiceItem
	for _, line := range si.Lines.Values {
		items = append(items, SerializeStripeInvoiceItem(line))
	}
	inv.Items = items
	return inv
}

// stateFromStripe attempts to map Stripe attributes to db state.
// Not covered due to different semantics: invoice.StateNotPayable,
// invoice.StateDeferred
func stateFromStripe(si *stripe.Invoice) string {
	if si.Paid {
		return invoice.StatePaid
	} else {
		if si.NextAttempt == 0 {
			return invoice.StateFailed
		}
		return invoice.StateUnpaid
	}
}
