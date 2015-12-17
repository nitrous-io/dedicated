package serializers_test

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("image serializer", func() {
	var (
		builtAt   = time.Date(2013, time.October, 1, 0, 0, 0, 0, time.UTC)
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
			IsSnapshot:  false,
			ActualSize:  111111,
			VirtualSize: 222222,
			BuiltAt:     &builtAt,
			CreatedAt:   &createdAt,
			UpdatedAt:   &updatedAt,
		}
	)

	Describe("SerializeImage", func() {
		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeImage(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON((fmt.Sprintf(`{
				"id": 10,
				"user_id": 11,
				"host_id": 12,
				"freighter_id": "cafebabe",
				"name": "scratch image",
				"slug": "scratch",
				"tag": "latest",
				"is_snapshot": false,
				"stack": "%s",
				"repo": "scratch:latest",
				"actual_size": 111111,
				"virtual_size": 222222,
				"built_at": "2013-10-01T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`, model.Stack()))))
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
			Expect(b).To(MatchJSON((fmt.Sprintf(`[{
				"id": 10,
				"user_id": 11,
				"host_id": 12,
				"freighter_id": "cafebabe",
				"name": "scratch image",
				"slug": "scratch",
				"tag": "latest",
				"is_snapshot": false,
				"stack": "%s",
				"repo": "scratch:latest",
				"actual_size": 111111,
				"virtual_size": 222222,
				"built_at": "2013-10-01T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}]`, model.Stack()))))
		})
	})
})
