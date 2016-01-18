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
}

type InvoiceSerialized struct {
	ID          int64                    `json:"id"`
	UserID      int64                    `json:"user_id"`
	Amount      int64                    `json:"amount"`
	PeriodStart *time.Time               `json:"period_start"`
	State       string                   `json:"state"`
	Items       []*InvoiceItemSerialized `json:"items"`
	Description string                   `json:"description"`

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
		Description: n.Description(),

		// TODO: remove deprecated fields below once frontend is updated
		TotalCharge: n.Amount,
		PeriodEnd:   &pstart,
	}
	inv.Items = n.SerializeItems()
	return inv
}

func (n NitrousInvoice) SerializeItems() []*InvoiceItemSerialized {
	switch n.Type() {
	case NinvoiceJuly1:
		return []*InvoiceItemSerialized{
			{
				StripeID:    fmt.Sprintf("%s-line1", n.ID),
				Description: n.Description(),
				Amount:      0,
			},
		}
	case NinvoiceFreeTrial:
		return []*InvoiceItemSerialized{
			{
				StripeID:    fmt.Sprintf("%s-line1", n.ID),
				Description: n.Description(),
				Amount:      0,
			},
		}
	}

	items := []*InvoiceItemSerialized{}
	for i, line := range n.Lines.Values {
		desc := line.Desc
		if len(n.Lines.Values) == 1 && line.Desc == "" {
			desc = n.Description()
		}
		items = append(items, &InvoiceItemSerialized{
			StripeID:    fmt.Sprintf("%s-line%d", n.ID, i+1),
			Description: desc,
			Amount:      line.Amount,

			// TODO: remove deprecated fields below once frontend is updated
			TotalCharge: line.Amount,
		})
	}

	// for partially-credited subscription invoice we need to add in the line
	// item showing the credit
	if n.Type() == NinvoiceCreditedSubPartial {
		creditVal := n.Total - n.Amount
		items = append(items, &InvoiceItemSerialized{
			StripeID:    fmt.Sprintf("%s-line%d", n.ID, len(n.Lines.Values)+1),
			Description: fmt.Sprintf("Credit of $%.2f", float64(creditVal)/100.0),
			Amount:      -creditVal,

			// TODO: remove deprecated fields below once frontend is updated
			TotalCharge: -creditVal,
		})
	}

	return items
}

func (n NitrousInvoice) Description() string {
	t, plan, isSub := n.Type(), n.plan(), n.IsSubscription()
	switch {
	case t == NinvoiceJuly1:
		return "Complimentary subscription for month of July 2015"
	case t == NinvoiceFreeTrial:
		pstart, pend := time.Unix(n.Lines.Values[0].Period.Start, 0), time.Unix(n.Lines.Values[0].Period.End, 0)
		dur := pend.Sub(pstart)
		return fmt.Sprintf("Free trial (%.0f days)", dur.Hours()/24.0)
	case isSub && plan != nil:
		return fmt.Sprintf("Subscription to %s: $%.0f/%s", plan.Name, float64(plan.Amount)/100.00, plan.Interval)
	case !isSub:
		pstart := time.Unix(n.Start, 0)
		return fmt.Sprintf("Usage for %s", pstart.Format("Jan 2006"))
	}

	pstart, pend := time.Unix(n.Start, 0), time.Unix(n.End, 0)
	pstartStr, pendStr := pstart.Format("Jan 2 2006"), pend.Format("Jan 2 2006")

	// Given the invoices we expect, this should not be reached.  But if we do get
	// here, fallback to a generic description
	return fmt.Sprintf("Invoice for %s to %s", pstartStr, pendStr)
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
		if len(n.Lines.Values) == 1 && n.Lines.Values[0].Period != nil {
			return NinvoiceFreeTrial
		}
	}

	// remaining possibilities are successful usage-based or successful sub
	if n.IsSubscription() {
		return NinvoiceSuccessSub
	}

	// Note: as this is the final condition, any invoice type that is
	// unaccounted for will be marked as this type of invoice
	return NinvoiceSuccessUsage
}

func (n NitrousInvoice) plan() *stripe.Plan {
	for _, line := range n.Lines.Values {
		if line.Plan != nil {
			return line.Plan
		}
	}
	return nil
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
