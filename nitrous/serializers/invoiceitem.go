package serializers

import (
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
)

type InvoiceItem struct {
	ID                 int64   `json:"id"`
	InvoiceID          int64   `json:"invoice_id"`
	HostID             int64   `json:"host_id"`
	HostSlug           string  `json:"host_slug"`
	Hours              int64   `json:"hours"`
	Terminated         bool    `json:"terminated"`
	HourlyCents        float64 `json:"hourly_cents"`
	TotalCharge        int64   `json:"total_charge"`
	TotalChargeDollars float64 `json:"total_charge_dollars"`
	Description        string  `json:"description"`
}

func SerializeInvoiceItem(i *invoiceitem.InvoiceItem) *InvoiceItem {
	if i == nil {
		return nil
	}
	return &InvoiceItem{
		ID:                 i.ID,
		InvoiceID:          i.InvoiceID,
		HostID:             i.HostID.Int64,
		Hours:              i.Hours.Int64,
		Terminated:         i.Terminated,
		HourlyCents:        i.HourlyCents(),
		TotalCharge:        i.TotalCharge(),
		TotalChargeDollars: float64(i.TotalCharge()) / 100.0,
	}
}
