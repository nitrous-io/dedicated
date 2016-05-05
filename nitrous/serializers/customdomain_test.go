package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/customdomain"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("custom domain serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &customdomain.CustomDomain{
			ID:            10,
			ContainerID:   11,
			Domain:        "www.gomama.com",
			Port:          80,
			ContainerPort: 4000,
			CreatedAt:     &createdAt,
			UpdatedAt:     &updatedAt,
		}
	)

	Describe("SerializeCustomDomain", func() {
		It("serializes to JSON", func() {
			d := serializers.SerializeCustomDomain(model)
			b, err := json.Marshal(d)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"container_id": 11,
				"domain": "www.gomama.com",
				"port": 80,
				"container_port": 4000,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})
	})

	Describe("SerializeCustomDomains", func() {
		It("serializes to JSON", func() {
			d := serializers.SerializeCustomDomains([]*customdomain.CustomDomain{model})
			b, err := json.Marshal(d)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[{
				"id": 10,
				"container_id": 11,
				"domain": "www.gomama.com",
				"port": 80,
				"container_port": 4000,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}]`))
		})
	})
})
