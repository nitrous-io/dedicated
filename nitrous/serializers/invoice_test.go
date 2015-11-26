package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/stripememo"
	"github.com/nitrous-io/dedicated/nitrous/serializers"

	"github.com/nitrous-io/dedicated/nitrous/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

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

		It("serializes to JSON", func() {
			r := serializers.SerializeStripeInvoice(model)
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
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
				"items": null
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
					"invoice_id": 0,
					"item_type": "compute",
					"host_id": 0,
					"host_slug": "",
					"hours": 0,
					"terminated": false,
					"hourly_cents": 0,
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
