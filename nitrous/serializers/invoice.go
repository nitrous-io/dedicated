package serializers

import (
	"time"

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
	if i.StripeID.Valid {
		inv.StripeID = i.StripeID.String
	}
	return inv
}

func SerializeStripeInvoice(mi *stripememo.Invoice) *stripememo.InvoiceSerialized {
	if mi == nil {
		return nil
	}
	si, err := mi.StripeInvoice()
	if err != nil {
		return nil
	}
	ni := &stripememo.NitrousInvoice{*si}
	return ni.Serialize()
}
