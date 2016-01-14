package serializers_test

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	stripe "github.com/stripe/stripe-go"

	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/stripememo"
	"github.com/nitrous-io/dedicated/nitrous/serializers"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var stripeInvoices = map[string]*stripe.Invoice{}

var _ = BeforeSuite(func() {
	p, err := filepath.Abs("./stripe-invoices.cjson")
	Expect(err).To(BeNil())
	stripeInvoices, err = parseInvoiceFixtures(p)
	Expect(err).To(BeNil())
})

var _ = Describe("invoice serializer", func() {
	Describe("SerializeStripeInvoice", func() {
		var (
			model *stripememo.Invoice
		)

		BeforeEach(func() {
			model = &stripememo.Invoice{
				BillingProfileID: 123,
				UserID:           456,
				InvoiceJSON:      test.StripeInvoiceResponse,
			}
		})

		It("serializes to JSON with description when invoice item is a subscription", func() {
			r := serializers.SerializeStripeInvoice(model)
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			// this also tests invoiceitem serialization
			Expect(b).To(MatchJSON(`{
				"id": 0,
				"sid": "in_17AYBv2eZvKYlo2C8sW8Cs4R",
				"user_id": 456,
				"total_charge": 3000,
				"state": "unpaid",
				"period_start": "2015-11-23T04:12:51Z",
				"period_end": "2015-11-24T04:12:51Z",
				"attempted": false,
				"total_credit": 0,
				"available_credit": 0,
				"subscription": true,
				"items": [
					{
						"id": 0,
						"sid": "sub_7PNezyXAgzNnkn",
						"description": "Subscription to New plan name ($20/month)",
						"total_charge": 2000,
						"usage": null
					}
				]
			}`))
		})

		It("serializes to JSON with given invoice item description if present", func() {
			si, err := model.StripeInvoice()
			Expect(err).To(BeNil())
			si.Lines.Values[0].Desc = "Credit of $9.00"
			b, err := json.Marshal(si)
			Expect(err).To(BeNil())
			model.InvoiceJSON = string(b)

			r := serializers.SerializeStripeInvoice(model)
			b, err = json.Marshal(r)
			Expect(err).To(BeNil())
			// this also tests invoiceitem serialization
			Expect(b).To(MatchJSON(`{
				"id": 0,
				"sid": "in_17AYBv2eZvKYlo2C8sW8Cs4R",
				"user_id": 456,
				"total_charge": 3000,
				"state": "unpaid",
				"period_start": "2015-11-23T04:12:51Z",
				"period_end": "2015-11-24T04:12:51Z",
				"attempted": false,
				"total_credit": 0,
				"available_credit": 0,
				"subscription": true,
				"items": [
					{
						"id": 0,
						"sid": "sub_7PNezyXAgzNnkn",
						"description": "Credit of $9.00",
						"total_charge": 2000,
						"usage": null
					}
				]
			}`))
		})
	})

	Describe("SerializeInvoice", func() {
		var (
			model *invoice.Invoice
			t1    = time.Unix(20, 0)
			t2    = t1.Add(1 * time.Hour)
		)
		BeforeEach(func() {
			model = &invoice.Invoice{
				ID:          23,
				UserID:      29,
				TotalCharge: 329,
				PeriodStart: &t1,
				PeriodEnd:   &t2,
				State:       invoice.StatePaid,
				Attempts:    1,
			}
		})

		It("serializes to JSON", func() {
			r := serializers.SerializeInvoice(model)
			r.TotalCredit = 100
			r.AvailableCredit = 200
			usage := &serializers.HostUsage{
				Minutes: 193,
				Start:   t1,
				End:     t2,
			}
			item1 := serializers.InvoiceItem{
				ID:       12,
				ItemType: "compute",
				Usage:    []*serializers.HostUsage{usage},
			}
			r.Items = []serializers.InvoiceItem{item1}
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 23,
				"user_id": 29,
				"total_charge": 329,
				"period_start": "1970-01-01T00:00:20Z",
				"period_end": "1970-01-01T01:00:20Z",
				"state": "paid",
				"attempted": true,
				"total_credit": 100,
				"available_credit": 200,
				"subscription": false,
				"items": [{
					"id": 12,
					"item_type": "compute",
					"total_charge": 0,
					"description": "",
					"usage": [{
						"minutes": 193,
						"start_at": "1970-01-01T00:00:20Z",
						"end_at": "1970-01-01T01:00:20Z"
					}]
				}]
			}`))
		})

		It("should indicate if a subscription is present", func() {
			model.SubscriptionID = sql.NullInt64{2, true}
			r := serializers.SerializeInvoice(model)
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 23,
				"user_id": 29,
				"total_charge": 329,
				"period_start": "1970-01-01T00:00:20Z",
				"period_end": "1970-01-01T01:00:20Z",
				"state": "paid",
				"attempted": true,
				"total_credit": 0,
				"available_credit": 0,
				"subscription": true,
				"items": null
		    }`))
		})
	})
})

var invoiceIDToKey = map[string]string{
	"in_16JckrD2OOBduWpzD4jRuR5B": "july01",
	"in_17L56DD2OOBduWpz7PwZ7BNf": "freetrial",
	"in_17OGrhD2OOBduWpzsNYZKqrK": "success-sub",
	"in_168gMyD2OOBduWpzJlqG8C9J": "success-usage",
	"in_17Q9pMD2OOBduWpzu7hqP1rX": "failed-closed",
	"in_17SI46D2OOBduWpzgi38TLz5": "failed-due", // see note in cjson file
	"in_16QsLUD2OOBduWpzs5RpLvKx": "credited-sub-full",
	"in_16QszVD2OOBduWpzbrn3YbyR": "credited-sub-partial",
	"in_15RBORD2OOBduWpze7Gef6b0": "credited-usage-partial",
}

// parses a 'cjson' format file: json with lines that start with '//' treated
// as comments
func parseInvoiceFixtures(filepath string) (map[string]*stripe.Invoice, error) {
	m := map[string]*stripe.Invoice{}
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	lines := []string{}
	for s.Scan() {
		t := s.Text()
		if strings.HasPrefix(t, "//") {
			continue
		}
		lines = append(lines, t)
	}
	jsonStr := strings.Join(lines, "")
	invoices := []*stripe.Invoice{}
	if err := json.Unmarshal([]byte(jsonStr), &invoices); err != nil {
		return nil, err
	}
	for _, inv := range invoices {
		m[invoiceIDToKey[inv.ID]] = inv
	}
	return m, nil
}
