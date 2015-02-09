package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("invoice serializer", func() {
	Describe("SerializeInvoice", func() {
		var t1 = time.Unix(20, 0)
		var t2 = t1.Add(1 * time.Hour)
		var item1 = serializers.InvoiceItem{
			ID: 12,
		}

		var model = &invoice.Invoice{
			ID:               23,
			UserID:           29,
			TotalCharge:      329,
			PeriodStart:      &t1,
			PeriodEnd:        &t2,
			SuccessfulCharge: false,
		}

		It("serializes to JSON", func() {
			r := serializers.SerializeInvoice(model)
			r.Items = []serializers.InvoiceItem{item1}
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
        "id": 23,
        "user_id": 29,
        "total_charge": 329,
        "total_charge_dollars": 3.29,
        "period_start": "1970-01-01T00:00:20Z",
        "period_end": "1970-01-01T01:00:20Z",
        "successful_charge": false,
        "items": [{
          "id": 12,
          "invoice_id": 0,
          "host_id": 0,
          "host_slug": "",
          "hours": 0,
          "terminated": false,
          "hourly_cents": 0,
          "total_charge": 0,
          "total_charge_dollars": 0.0,
          "description": ""
        }]
      }`))
		})
	})
})
