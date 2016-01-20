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
			"description": "Complimentary subscription for month of July 2015",
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

	It("serializes free trial invoice", func() {
		inv := NewNinvoice(stripememo.NinvoiceFreeTrial)
		j, err := jsonize(inv)
		Expect(err).To(BeNil())
		Expect(j).To(MatchJSON(`{
			"id": 0,
			"user_id": 0,
			"amount": 0,
			"period_start": "2015-12-23T05:23:45Z",
			"state": "paid",
			"description": "Free trial (14 days)",
			"items":[{
				"amount": 0,
				"description": "Free trial (14 days)",
				"sid": "in_17L56DD2OOBduWpz7PwZ7BNf-line1",
				"total_charge": 0
			}],
			"sid": "in_17L56DD2OOBduWpz7PwZ7BNf",
			"total_charge": 0,
			"period_end": "2015-12-23T05:23:45Z"
		}`))
	})

	It("serializes successful subscription invoice", func() {
		inv := NewNinvoice(stripememo.NinvoiceSuccessSub)
		j, err := jsonize(inv)
		Expect(err).To(BeNil())
		Expect(j).To(MatchJSON(`{
			"id": 0,
			"user_id": 0,
			"amount": 2900,
			"period_start": "2016-01-01T00:33:57Z",
			"state": "paid",
			"description": "Subscription to Standard (Early-2015): $29/month",
			"items":[{
				"amount": 2900,
				"description": "Subscription to Standard (Early-2015): $29/month",
				"sid": "in_17OGrhD2OOBduWpzsNYZKqrK-line1",
				"total_charge": 2900
			}],
			"sid": "in_17OGrhD2OOBduWpzsNYZKqrK",
			"total_charge": 2900,
			"period_end": "2016-01-01T00:33:57Z"
		}`))
	})

	It("serializes successful usage-based invoice", func() {
		inv := NewNinvoice(stripememo.NinvoiceSuccessUsage)
		j, err := jsonize(inv)
		Expect(err).To(BeNil())
		Expect(j).To(MatchJSON(`{
			"id": 0,
			"user_id": 0,
			"amount": 3696,
			"period_start": "2015-06-01T00:01:32Z",
			"state": "paid",
			"description": "Usage for May 2015",
			"items":[{
 				"amount":600,
				"description":"'ronoaldop-1734' base (50 GB storage) - 672 hours at 0.893c/hour",
				"sid":"in_168gMyD2OOBduWpzJlqG8C9J-line1",
				"total_charge":600
			}, {
				"amount":2399,
				"description":"'ronoaldop-1734' compute (2 GB memory) - 672 hours at 3.57c/hour",
				"sid":"in_168gMyD2OOBduWpzJlqG8C9J-line2",
				"total_charge":2399
			},{
				"amount":140,
				"description":"'sulamerica-2868' base (50 GB storage) - 156 hours at 0.893c/hour (terminated)",
				"sid":"in_168gMyD2OOBduWpzJlqG8C9J-line3",
				"total_charge":140},{
					"amount":557,
					"description":"'sulamerica-2868' compute (2 GB memory) - 156 hours at 3.57c/hour (terminated)",
					"sid":"in_168gMyD2OOBduWpzJlqG8C9J-line4",
					"total_charge":557
				}
			],
			"sid": "in_168gMyD2OOBduWpzJlqG8C9J",
			"total_charge": 3696,
			"period_end": "2015-06-01T00:01:32Z"
		}`))
	})

	It("serializes failed and closed invoice", func() {
		inv := NewNinvoice(stripememo.NinvoiceFailedClosed)
		j, err := jsonize(inv)
		Expect(err).To(BeNil())
		// json under test: ensure state is 'failed'
		Expect(j).To(MatchJSON(`{
			"id": 0,
			"user_id": 0,
			"amount": 3900,
			"period_start": "2016-01-06T05:27:20Z",
			"state": "failed",
			"description": "Subscription to Standard: $39/month",
			"items":[{
				"amount": 3900,
				"description": "Subscription to Standard: $39/month",
				"sid": "in_17Q9pMD2OOBduWpzu7hqP1rX-line1",
				"total_charge": 3900
			}],
			"sid": "in_17Q9pMD2OOBduWpzu7hqP1rX",
			"total_charge": 3900,
			"period_end": "2016-01-06T05:27:20Z"
		}`))
	})

	It("serializes failed and due invoice", func() {
		inv := NewNinvoice(stripememo.NinvoiceFailedDue)
		j, err := jsonize(inv)
		Expect(err).To(BeNil())
		// json under test: ensure state is 'unpaid'
		Expect(j).To(MatchJSON(`{
			"id": 0,
			"user_id": 0,
			"amount": 1900,
			"period_start": "2016-01-12T02:39:22Z",
			"state": "unpaid",
			"description": "Subscription to Basic: $19/month",
			"items":[{
				"amount": 1900,
				"description": "Subscription to Basic: $19/month",
				"sid": "in_17SI46D2OOBduWpzgi38TLz5-line1",
				"total_charge": 1900
			}],
			"sid": "in_17SI46D2OOBduWpzgi38TLz5",
			"total_charge": 1900,
			"period_end": "2016-01-12T02:39:22Z"
		}`))
	})

	It("serializes partially credited subscription invoice", func() {
		inv := NewNinvoice(stripememo.NinvoiceCreditedSubPartial)
		j, err := jsonize(inv)
		Expect(err).To(BeNil())
		Expect(j).To(MatchJSON(`{
			"id": 0,
			"user_id": 0,
			"amount": 1900,
			"period_start": "2015-07-21T05:08:33Z",
			"state": "paid",
			"description": "Subscription to Standard (Mid-2015): $29/month",
			"items":[{
				"amount": 2900,
				"description": "Subscription to Standard (Mid-2015): $29/month",
				"sid": "in_16QszVD2OOBduWpzbrn3YbyR-line1",
				"total_charge": 2900
			},{
				"amount": -1000,
				"description": "Credit of $10.00",
				"sid": "in_16QszVD2OOBduWpzbrn3YbyR-line2",
				"total_charge": -1000
			}],
			"sid": "in_16QszVD2OOBduWpzbrn3YbyR",
			"total_charge": 1900,
			"period_end": "2015-07-21T05:08:33Z"
		}`))
	})

	It("serializes partially credited usage invoice", func() {
		inv := NewNinvoice(stripememo.NinvoiceCreditedUsagePartial)
		j, err := jsonize(inv)
		Expect(err).To(BeNil())
		// json under test: ensure state is 'unpaid'
		Expect(j).To(MatchJSON(`{
			"id": 0,
			"user_id": 0,
			"amount": 248,
			"period_start": "2015-02-01T00:15:15Z",
			"state": "paid",
			"description": "Usage for Jan 2015",
			"items":[{
				"amount": -80,
				"description": "Credit of $0.80",
				"sid": "in_15RBORD2OOBduWpze7Gef6b0-line1",
				"total_charge": -80
			},{
				"amount": 328,
				"description": "Application 'bitomule-7892' 88 hours at 3.721c/hour",
				"sid": "in_15RBORD2OOBduWpze7Gef6b0-line2",
				"total_charge": 328
			}],
			"sid": "in_15RBORD2OOBduWpze7Gef6b0",
			"total_charge": 248,
			"period_end": "2015-02-01T00:15:15Z"
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
