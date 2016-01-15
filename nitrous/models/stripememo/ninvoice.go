package stripememo

import (
	"fmt"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go"

	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
)

// NitrousInvoice is a wrapper struct for us to have methods on Stripe invoices
type NitrousInvoice struct {
	stripe.Invoice

	Description string
}

type InvoiceSerialized struct {
	ID          int64                    `json:"id"`
	UserID      int64                    `json:"user_id"`
	Amount      int64                    `json:"amount"`
	PeriodStart *time.Time               `json:"period_start"`
	State       string                   `json:"state"`
	Items       []*InvoiceItemSerialized `json:"items"`

	// this is used to provide a unique id for frontend
	StripeID string `json:"sid,omitempty"`

	// TODO remove deprecated fields once frontend is updated:
	TotalCharge int64      `json:"total_charge"`
	PeriodEnd   *time.Time `json:"period_end"`
}

type InvoiceItemSerialized struct {
	Amount      int64  `json:"amount"`
	Description string `json:"description"`

	// this is used to provide a unique id for frontend
	StripeID string `json:"sid,omitempty"`

	// TODO remove deprecated fields once frontend is updated:
	TotalCharge int64 `json:"total_charge"`
}

// types of invoices
const (
	// upon changing from usage-based to subscriptions in July 2015, we comped
	// all existing users with a free month of subscription
	NinvoiceJuly1 = "july01"

	NinvoiceFreeTrial    = "freetrial"
	NinvoiceSuccessSub   = "success-sub"
	NinvoiceSuccessUsage = "success-usage"
	NinvoiceFailedClosed = "failed-closed"

	// the difference between this and "failed-closed" is that this type of
	// invoice has not gone through the maximum number of attempts.  Once that
	// happens, it will transition to "failed-closed"
	NinvoiceFailedDue = "failed-due"

	NinvoiceCreditedSubFull      = "credited-sub-full"
	NinvoiceCreditedSubPartial   = "credited-sub-partial"
	NinvoiceCreditedUsagePartial = "credited-usage-partial"
)

func (n NitrousInvoice) Serialize() *InvoiceSerialized {
	// Note: the UserID field will have to be added later since there is no db
	// access at this point

	pstart := time.Unix(n.Start, 0)
	inv := &InvoiceSerialized{
		StripeID:    n.ID,
		Amount:      n.Amount,
		State:       n.State(),
		PeriodStart: &pstart,

		// TODO: remove deprecated fields below once frontend is updated
		TotalCharge: n.Amount,
		PeriodEnd:   &pstart,
	}
	inv.Items = n.SerializeItems()
	return inv
}

func (n NitrousInvoice) SerializeItems() []*InvoiceItemSerialized {
	if n.Type() == NinvoiceJuly1 {
		return []*InvoiceItemSerialized{
			{
				StripeID:    fmt.Sprintf("%s-line1", n.ID),
				Description: "Complimentary subscription for month of July 2015",
				Amount:      0,
			},
		}
	}

	items := []*InvoiceItemSerialized{}
	for i, line := range n.Lines.Values {
		items = append(items, &InvoiceItemSerialized{
			StripeID:    fmt.Sprintf("%s-line%d", n.ID, i),
			Description: "foo",
			Amount:      line.Amount,

			// TODO: remove deprecated fields below once frontend is updated
			TotalCharge: line.Amount,
		})
	}
	return items
}

func (n NitrousInvoice) IsSubscription() bool {
	for _, line := range n.Lines.Values {
		if line.Type == "subscription" {
			return true
		}
	}
	return false
}

func (n NitrousInvoice) HasCreditLineItem() bool {
	for _, line := range n.Lines.Values {
		if strings.HasPrefix(line.Desc, "Credit of") {
			return true
		}
	}
	// note that a partially credited subscription invoice has no credit line item
	return false
}

// ShowInHistory returns true if a user should see that invoice in their billing history
func (n NitrousInvoice) ShowInHistory() bool {
	return n.Attempted &&
		n.Type() != NinvoiceCreditedSubFull
}

func (n NitrousInvoice) State() string {
	if n.Paid {
		return invoice.StatePaid
	} else {
		if n.NextAttempt == 0 {
			return invoice.StateFailed
		}
		return invoice.StateUnpaid
	}
}

func (n NitrousInvoice) Type() string {
	if !n.Paid {
		if n.Closed {
			return NinvoiceFailedClosed
		} else {
			return NinvoiceFailedDue
		}
	}

	if n.HasCreditLineItem() {
		if n.Amount == 0 {
			return NinvoiceCreditedSubFull
		} else {
			return NinvoiceCreditedUsagePartial
		}
	}

	// a partially credited subscription has no credit line item, but will have amount due less than total
	// this condition is shared by NinvoiceCreditedSubFull, but that case is handled above
	if n.Amount < n.Total {
		return NinvoiceCreditedSubPartial
	}

	// n.Amount == 0 is shared by NinvoiceCreditedSubFull, but that case is handled above
	if n.Amount == 0 && n.IsSubscription() {
		if t := time.Unix(n.Date, 0); t.Format("2006-01-02") == "2015-07-01" {
			return NinvoiceJuly1
		}
		return NinvoiceFreeTrial
	}

	// remaining possibilities are successful usage-based or successful sub
	if n.IsSubscription() {
		return NinvoiceSuccessSub
	}

	// Note: as this is the final condition, any invoice type that is
	// unaccounted for will be marked as this type of invoice
	return NinvoiceSuccessUsage
}
