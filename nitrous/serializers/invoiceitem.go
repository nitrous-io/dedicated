package serializers

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	stripe "github.com/stripe/stripe-go"

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

func SerializeStripeInvoiceItem(siItem *stripe.InvoiceLine) InvoiceItem {
	return InvoiceItem{
		// No Stripe equivalent for all fields apart from charge and description
		StripeID:    siItem.ID,
		Description: desc(siItem),
		TotalCharge: siItem.Amount,
	}
}

func desc(siItem *stripe.InvoiceLine) string {
	// Credit and old usage-based line items have descriptions
	// Subscriptions do not
	if siItem.Desc != "" {
		return siItem.Desc
	}
	if siItem.Plan == nil {
		logrus.WithFields(logrus.Fields{
			"line_id":   siItem.ID,
			"mailalert": true,
		}).Errorf("Unexpected invoice line")
		return ""
	}
	return fmt.Sprintf("Subscription to %s ($%s/%s)",
		siItem.Plan.Name, currency(siItem.Plan.Amount), siItem.Plan.Interval)
}

func currency(amt uint64) string {
	if amt%100 == 0 {
		return fmt.Sprintf("%d", amt/100)
	} else {
		return fmt.Sprintf("%0.2f", float64(amt)/100.0)
	}
}
