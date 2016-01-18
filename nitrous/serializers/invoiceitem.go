package serializers

import (
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
)

type InvoiceItem struct {
	ID          int64        `json:"id"`
	InvoiceID   int64        `json:"invoice_id,omitempty"`
	ItemType    string       `json:"item_type,omitempty"`
	HostID      int64        `json:"host_id,omitempty"`
	HostSlug    string       `json:"host_slug,omitempty"`
	Hours       int64        `json:"hours,omitempty"`
	Terminated  bool         `json:"terminated,omitempty"`
	HourlyCents float64      `json:"hourly_cents,omitempty"`
	TotalCharge int64        `json:"total_charge"`
	Description string       `json:"description"`
	Usage       []*HostUsage `json:"usage"`

	StripeID string `json:"sid,omitempty"`
}

func SerializeInvoiceItem(i *invoiceitem.InvoiceItem) *InvoiceItem {
	if i == nil {
		return nil
	}
	return &InvoiceItem{
		ID:          i.ID,
		InvoiceID:   i.InvoiceID,
		ItemType:    i.ItemType,
		HostID:      i.HostID.Int64,
		Hours:       i.Hours.Int64,
		Terminated:  i.Terminated,
		HourlyCents: i.HourlyCents(),
	}
}
