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
		publishedAt = time.Date(2013, time.November, 3, 0, 0, 0, 0, time.UTC)
		createdAt   = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt   = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model *snapshot.Snapshot
	)

	BeforeEach(func() {
		model = &snapshot.Snapshot{
			ID:            10,
			ImageID:       11,
			UserID:        12,
			OrgID:         sql.NullInt64{13, true},
			Slug:          "nitrous-io/nitrous-react-dev",
			Namespace:     "nitrous-io",
			Name:          "nitrous-react-dev",
			Tag:           "latest",
			DisplayName:   "nitrous-react dev",
			OriginalImage: "quay.io/nitrous/nodejs:latest",
			Size:          sql.NullInt64{1024, true},
			Downloads:     42,
			PublishedAt:   pq.NullTime{publishedAt, true},
			CreatedAt:     pq.NullTime{createdAt, true},
			UpdatedAt:     pq.NullTime{updatedAt, true},
		}
	})

	Describe("SerializeSnapshot", func() {
		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeSnapshot(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"image_id": 11,
				"user_id": 12,
				"org_id": 13,
				"slug": "nitrous-io/nitrous-react-dev",
				"display_name": "nitrous-react dev",
				"stack": "%s",
				"size": 1024,
				"downloads": 42,
				"published_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`, model.Stack())))
		})

		It("omits size if empty", func() {
			model.Size = sql.NullInt64{}

			b, err := json.Marshal(serializers.SerializeSnapshot(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"image_id": 11,
				"user_id": 12,
				"org_id": 13,
				"slug": "nitrous-io/nitrous-react-dev",
				"display_name": "nitrous-react dev",
				"stack": "%s",
				"downloads": 42,
				"published_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`, model.Stack())))
		})

		It("omits published_at if empty", func() {
			model.PublishedAt = pq.NullTime{}

			b, err := json.Marshal(serializers.SerializeSnapshot(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"image_id": 11,
				"user_id": 12,
				"org_id": 13,
				"slug": "nitrous-io/nitrous-react-dev",
				"display_name": "nitrous-react dev",
				"size":1024,
				"downloads": 42,
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
				"slug": "nitrous-io/nitrous-react-dev",
				"display_name": "nitrous-react dev",
				"stack": "%s",
				"size": 1024,
				"downloads": 42,
				"published_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}]`, model.Stack())))
		})
	})

	Describe("SerializeSnapshotWithOwner", func() {
		It("serializes to JSON", func() {
			model := &snapshot.SnapshotWithOwner{
				Snapshot:      model,
				OwnerUsername: sql.NullString{"aroonthampy", true},
				OwnerName:     sql.NullString{"Aroon Thampy", true},
				OwnerEmail:    sql.NullString{"aroonthampy@gmail.com", true},
			}

			b, err := json.Marshal(serializers.SerializeSnapshotWithOwner(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 10,
				"image_id": 11,
				"user_id": 12,
				"org_id": 13,
				"slug": "nitrous-io/nitrous-react-dev",
				"display_name": "nitrous-react dev",
				"stack": "%s",
				"size": 1024,
				"downloads": 42,
				"owner_username": "aroonthampy",
				"owner_name": "Aroon Thampy",
				"owner_email": "aroonthampy@gmail.com",
				"published_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`, model.Stack())))
		})

		It("serializes to null JSON when snapshot is nil", func() {
			b, err := json.Marshal(serializers.SerializeSnapshotWithOwner(nil))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`null`))
		})
	})

	Describe("SerializeSnapshots", func() {
		It("serializes to JSON", func() {
			model := &snapshot.SnapshotWithOwner{
				Snapshot:      model,
				OwnerUsername: sql.NullString{"aroonthampy", true},
				OwnerName:     sql.NullString{"Aroon Thampy", true},
				OwnerEmail:    sql.NullString{"aroonthampy@gmail.com", true},
			}

			b, err := json.Marshal(serializers.SerializeSnapshotsWithOwner([]*snapshot.SnapshotWithOwner{model}))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`[{
				"id": 10,
				"image_id": 11,
				"user_id": 12,
				"org_id": 13,
				"slug": "nitrous-io/nitrous-react-dev",
				"display_name": "nitrous-react dev",
				"stack": "%s",
				"size": 1024,
				"downloads": 42,
				"owner_username": "aroonthampy",
				"owner_name": "Aroon Thampy",
				"owner_email": "aroonthampy@gmail.com",
				"published_at": "2013-11-03T00:00:00Z",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}]`, model.Stack())))
		})
	})
})
