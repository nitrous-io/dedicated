package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/nitrous/models/image"
	"github.com/nitrous-io/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("image serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &image.Image{
			ID:          10,
			UserID:      11,
			HostID:      12,
			FreighterID: "cafebabe",
			Name:        "scratch image",
			Slug:        "scratch",
			Tag:         "latest",
			ActualSize:  111111,
			VirtualSize: 222222,
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		}
	)

	Describe("SerializeImage", func() {
		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeImage(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": "cafebabe",
				"user_id": 11,
				"host_id": 12,
				"name": "scratch image",
				"slug": "scratch",
				"tag": "latest",
				"actual_size": 111111,
				"virtual_size": 222222,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})

		It("serializes to null JSON when image is nil", func() {
			b, err := json.Marshal(serializers.SerializeImage(nil))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`null`))
		})
	})

	Describe("SerializeImages", func() {
		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeImages([]*image.Image{model}))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[{
				"id": "cafebabe",
				"user_id": 11,
				"host_id": 12,
				"name": "scratch image",
				"slug": "scratch",
				"tag": "latest",
				"actual_size": 111111,
				"virtual_size": 222222,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}]`))
		})
	})
})
