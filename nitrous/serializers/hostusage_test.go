package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("host usage serializer", func() {
	Describe("SerializeHostUsage", func() {
		var t1 = time.Unix(20, 0)
		var t2 = t1.Add(1 * time.Hour)
		var model = &hosttransition.HostUsage{
			Minutes: 193,
			Start:   t1,
			End:     t2,
		}

		It("serializes to JSON", func() {
			r := serializers.SerializeHostUsage(model)
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
        "minutes": 193,
        "start_at": "1970-01-01T00:00:20Z",
        "end_at": "1970-01-01T01:00:20Z"
      }`))
		})
	})

	Describe("SerializeHostUsages", func() {
		var t1 = time.Unix(20, 0)
		var t2 = t1.Add(1 * time.Hour)
		var t3 = t2.Add(1 * time.Hour)
		var t4 = t3.Add(1 * time.Hour)
		var model1 = &hosttransition.HostUsage{
			Minutes: 193,
			Start:   t1,
			End:     t2,
		}
		var model2 = &hosttransition.HostUsage{
			Minutes: 180,
			Start:   t3,
			End:     t4,
		}

		It("returns a collection of serializers.HostUsage", func() {
			col := serializers.SerializeHostUsages([]*hosttransition.HostUsage{model1, model2})
			b, err := json.Marshal(col)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[{
        "minutes": 193,
        "start_at": "1970-01-01T00:00:20Z",
        "end_at": "1970-01-01T01:00:20Z"
      },{
        "minutes": 180,
        "start_at": "1970-01-01T02:00:20Z",
        "end_at": "1970-01-01T03:00:20Z"
      }]`))
		})
	})
})
