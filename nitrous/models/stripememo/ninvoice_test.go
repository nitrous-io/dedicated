package stripememo_test

import (
	"encoding/json"
	"fmt"

	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/stripememo"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func NewNinvoice(k string) *stripememo.NitrousInvoice {
	return &stripememo.NitrousInvoice{Invoice: *test.StripeInvoiceFixtures[k]}
}

func jsonize(inv *stripememo.NitrousInvoice) (string, error) {
	ser := inv.Serialize()
	bs, err := json.Marshal(ser)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

var _ = Describe("models/stripememo/ninvoice", func() {
	It("serializes July 1 2015 invoice", func() {
		inv := NewNinvoice(stripememo.NinvoiceJuly1)
		j, err := jsonize(inv)
		Expect(err).To(BeNil())
		Expect(j).To(MatchJSON(`{
			"id": 0,
			"user_id": 0,
			"amount": 0,
			"period_start": "2015-07-01T04:23:25Z",
			"state": "paid",
			"items":[{
				"amount": 0,
				"description": "Complimentary subscription for month of July 2015",
				"sid": "in_16JckrD2OOBduWpzD4jRuR5B-line1",
				"total_charge": 0
			}],
			"sid": "in_16JckrD2OOBduWpzD4jRuR5B",
			"total_charge": 0,
			"period_end": "2015-07-01T04:23:25Z"
		}`))
	})

	Context("State()", func() {
		var testcases = map[string]string{
			"july01":                 invoice.StatePaid,
			"freetrial":              invoice.StatePaid,
			"success-sub":            invoice.StatePaid,
			"success-usage":          invoice.StatePaid,
			"failed-closed":          invoice.StateFailed,
			"failed-due":             invoice.StateUnpaid,
			"credited-sub-full":      invoice.StatePaid,
			"credited-sub-partial":   invoice.StatePaid,
			"credited-usage-partial": invoice.StatePaid,
		}
		for k0, expected0 := range testcases {
			k, expected := k0, expected0
			It(fmt.Sprintf("is state '%s' for %s", expected, k), func() {
				ninvoice := NewNinvoice(k)
				Expect(ninvoice.State()).To(Equal(expected))
			})
		}
	})

	Context("Type()", func() {
		var testcases = []string{
			"july01",
			"freetrial",
			"success-sub",
			"success-usage",
			"failed-closed",
			"failed-due",
			"credited-sub-full",
			"credited-sub-partial",
			"credited-usage-partial",
		}
		for _, invType0 := range testcases {
			invType := invType0
			It(fmt.Sprintf("correctly identifies %s", invType), func() {
				ninvoice := NewNinvoice(invType)
				Expect(ninvoice.Type()).To(Equal(invType))
			})
		}
	})

	Context("IsSubscription()", func() {
		var testcases = map[string]bool{
			"july01":                 true,
			"freetrial":              true,
			"success-sub":            true,
			"success-usage":          false,
			"failed-closed":          true,
			"failed-due":             true,
			"credited-sub-full":      true,
			"credited-sub-partial":   true,
			"credited-usage-partial": false,
		}
		for k0, expected0 := range testcases {
			k, expected := k0, expected0
			It(fmt.Sprintf("is %b for %s", expected, k), func() {
				ninvoice := NewNinvoice(k)
				Expect(ninvoice.IsSubscription()).To(Equal(expected))
			})
		}
	})
})
