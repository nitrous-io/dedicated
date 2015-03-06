package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("invoice item serializer", func() {
	Describe("SerializeRegion", func() {
		var t1 = time.Unix(20, 0)
		var t2 = t1.Add(1 * time.Hour)
		var usage = &serializers.HostUsage{
			Minutes: 193,
			Start:   t1,
			End:     t2,
		}

		var model = &invoiceitem.InvoiceItem{
			ID:          23,
			InvoiceID:   28,
			HostID:      sql.NullInt64{39, true},
			Hours:       sql.NullInt64{12, true},
			Terminated:  true,
			HourlyPrice: sql.NullInt64{1000, true},
		}

		It("serializes to JSON", func() {
			r := serializers.SerializeInvoiceItem(model)
			r.HostSlug = "Slug1"
			r.Description = "I am a desc"
			r.TotalCharge = 12
			r.Usage = []*serializers.HostUsage{usage}
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
        "id": 23,
        "invoice_id": 28,
        "host_id": 39,
        "host_slug": "Slug1",
        "hours": 12,
        "terminated": true,
        "hourly_cents": 1,
        "total_charge": 12,
        "description": "I am a desc",
        "usage": [{
          "minutes": 193,
          "start_at": "1970-01-01T00:00:20Z",
          "end_at": "1970-01-01T01:00:20Z"
        }]
      }`))
		})
	})
})
