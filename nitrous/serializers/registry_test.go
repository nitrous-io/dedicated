package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/registry"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("registry serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &registry.Registry{
			ID:        10,
			UserID:    11, // Not serialized.
			HostID:    12,
			URL:       "https://quay.io/",
			Username:  "nitrousio",
			Email:     "devs@nitrous.io",
			CreatedAt: &createdAt, // Not serialized.
			UpdatedAt: &updatedAt, // Not serialized.
		}
	)

	Describe("SerializeRegistry", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializeRegistry(model)
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"host_id": 12,
				"url": "https://quay.io/",
				"username": "nitrousio",
				"email": "devs@nitrous.io"
			}`))
		})
	})

	Describe("SerializeRegistries", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializeRegistries([]*registry.Registry{model})
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[{
				"id": 10,
				"host_id": 12,
				"url": "https://quay.io/",
				"username": "nitrousio",
				"email": "devs@nitrous.io"
			}]`))
		})
	})
})
