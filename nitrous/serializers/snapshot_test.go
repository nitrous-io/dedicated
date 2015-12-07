package serializers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("snapshot serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &snapshot.Snapshot{
			ID:            10,
			ImageID:       11,
			UserID:        12,
			OrgID:         sql.NullInt64{13, true},
			Namespace:     "nitrous-io",
			Name:          "nitrous-react-dev",
			Tag:           "latest",
			Repository:    "nitrous-io/nitrous-react-dev",
			DisplayName:   "nitrous-react dev",
			OriginalImage: "quay.io/nitrous/nodejs:latest",
			CreatedAt:     pq.NullTime{createdAt, true},
			UpdatedAt:     pq.NullTime{updatedAt, true},
		}
	)

	Describe("SerializeSnapshot", func() {
		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeSnapshot(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"image_id": 11,
				"user_id": 12,
				"org_id": 13,
				"display_name": "nitrous-react dev",
				"stack": "%s",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`, model.Stack())))
		})

		It("serializes to null JSON when snapshot is nil", func() {
			b, err := json.Marshal(serializers.SerializeSnapshot(nil))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`null`))
		})
	})

	Describe("SerializeSnapshots", func() {
		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeSnapshots([]*snapshot.Snapshot{model}))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`[{
				"id": 10,
				"image_id": 11,
				"user_id": 12,
				"org_id": 13,
				"display_name": "nitrous-react dev",
				"stack": "%s",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}]`, model.Stack())))
		})
	})
})
