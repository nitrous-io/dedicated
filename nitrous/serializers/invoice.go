package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
)

type Invoice struct {
	ID                 int64         `json:"id"`
	UserID             int64         `json:"user_id"`
	TotalCharge        int64         `json:"total_charge"`
	TotalChargeDollars float64       `json:"total_charge_dollars"`
	PeriodStart        *time.Time    `json:"period_start"`
	PeriodEnd          *time.Time    `json:"period_end"`
	SuccessfulCharge   bool          `json:"successful_charge"`
	Items              []InvoiceItem `json:"items"`
}

func SerializeInvoice(i *invoice.Invoice) *Invoice {
	if i == nil {
		return nil
	}
	return &Invoice{
		ID:                 i.ID,
		UserID:             i.UserID,
		TotalCharge:        i.TotalCharge,
		PeriodStart:        i.PeriodStart,
		SuccessfulCharge:   i.SuccessfulCharge,
		PeriodEnd:          i.PeriodEnd,
		TotalChargeDollars: float64(i.TotalCharge) / 100.0,
	}
}
