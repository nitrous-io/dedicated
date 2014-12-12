package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = FDescribe("snapshot serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &snapshot.Snapshot{
			ID:        10,
			Size:      5,
			Url:       "foo",
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		}
	)

	Describe("SerializeSnapshot", func() {
		It("serializes to JSON", func() {
			v := serializers.SerializeSnapshot(model)
			b, err := json.Marshal(v)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"size": 5,
				"url": "foo",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})
	})

	Describe("SerializeSnapshots", func() {
		It("serializes to JSON", func() {
			vols := serializers.SerializeSnapshots([]*snapshot.Snapshot{model})
			b, err := json.Marshal(vols)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[{
				"id": 10,
				"size": 5,
				"url": "foo",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}]`))
		})
	})
})
