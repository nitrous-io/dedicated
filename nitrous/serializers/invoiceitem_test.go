package serializers_test

import (
	"encoding/json"

	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("invoice item serializer", func() {
	Describe("SerializeRegion", func() {
		var model = &invoiceitem.InvoiceItem{
			ID:          23,
			InvoiceID:   28,
			HostID:      39,
			Hours:       12,
			HourlyPrice: 1000,
		}

		It("serializes to JSON", func() {
			r := serializers.SerializeInvoiceItem(model)
			r.HostSlug = "Slug1"
			r.Description = "I am a desc"
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
        "id": 23,
        "invoice_id": 28,
        "host_id": 39,
        "host_slug": "Slug1",
        "hours": 12,
        "hourly_cents": 1,
        "total_charge": 12,
        "total_charge_dollars": 0.12,
        "description": "I am a desc"
      }`))
		})
	})
})
