package serializers

import (
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
)

type InvoiceItem struct {
	ID                 int64   `json:"id"`
	InvoiceID          int64   `json:"invoice_id"`
	HostID             int64   `json:"host_id"`
	HostSlug           string  `json:"host_slug"`
	Hours              int     `json:"hours"`
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
		HostID:             i.HostID,
		Hours:              i.Hours,
		HourlyCents:        i.HourlyCents(),
		TotalCharge:        i.TotalCharge(),
		TotalChargeDollars: float64(i.TotalCharge()) / 100.0,
	}
}
