package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
)

type Invoice struct {
	ID               int64         `json:"id"`
	UserID           int64         `json:"user_id"`
	TotalCharge      int64         `json:"total_charge"`
	PeriodStart      *time.Time    `json:"period_start"`
	PeriodEnd        *time.Time    `json:"period_end"`
	SuccessfulCharge bool          `json:"successful_charge"`
	Deferred         bool          `json:"deferred"`
	Failed           bool          `json:"failed"`
	Attempted        bool          `json:"attempted"`
	Items            []InvoiceItem `json:"items"`

	TotalCredit     int64 `json:"total_credit"`
	AvailableCredit int64 `json:"available_credit"`
}

func SerializeInvoice(i *invoice.Invoice) *Invoice {
	if i == nil {
		return nil
	}
	inv := &Invoice{
		ID:               i.ID,
		UserID:           i.UserID,
		TotalCharge:      i.TotalCharge,
		PeriodStart:      i.PeriodStart,
		SuccessfulCharge: i.SuccessfulCharge,
		Failed:           i.Failed,
		Deferred:         i.Deferred,
		PeriodEnd:        i.PeriodEnd,
		Attempted:        i.Attempts > 0,
	}
	if i.AvailableCredit.Valid {
		inv.AvailableCredit = i.AvailableCredit.Int64
	}
	return inv
}
