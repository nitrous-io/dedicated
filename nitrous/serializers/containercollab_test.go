package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("container collaborator (with user info) serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		sc *containercollab.ContainerCollabUser
	)

	BeforeEach(func() {
		sc = &containercollab.ContainerCollabUser{
			containercollab.ContainerCollab{
				ID:          10,
				UserID:      sql.NullInt64{29, true},
				ContainerID: 28,
				CreatedAt:   &createdAt,
				UpdatedAt:   &updatedAt,
			},
			sql.NullString{"mark", true},
			sql.NullString{"callum@callumj.com", true},
			sql.NullString{"Marky Mark", true},
		}
	})

	Describe("SerializeContainerCollabUser", func() {
		It("serializes to JSON", func() {
			s := serializers.SerializeContainerCollabUser(sc)
			b, err := json.Marshal(s)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
        "id": 10,
        "user_id": 29,
        "container_id": 28,
        "created_at": "2013-11-01T00:00:00Z",
        "updated_at": "2013-11-02T00:00:00Z",
        "username": "mark",
        "name": "Marky Mark",
        "gravatar_hash": "0b66d7eea76eca3623b6e5fe4f05536d"
      }`))
		})

		It("should include PendingEmail if InviteEmail is present", func() {
			sc.InviteEmail = sql.NullString{"abc@callumj.com", true}
			sc.Username = sql.NullString{Valid: false}
			sc.Email = sql.NullString{Valid: false}
			sc.Name = sql.NullString{Valid: false}

			s := serializers.SerializeContainerCollabUser(sc)
			b, err := json.Marshal(s)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
        "id": 10,
        "user_id": 29,
        "container_id": 28,
        "created_at": "2013-11-01T00:00:00Z",
        "updated_at": "2013-11-02T00:00:00Z",
        "pending_email": "abc@callumj.com",
        "gravatar_hash": "f322afb221b4b30267d62f46eff96cfa"
      }`))
		})
	})

	Describe("SerializeContainerCollabUsers", func() {
		It("serializes to JSON", func() {
			col := serializers.SerializeContainerCollabUsers([]*containercollab.ContainerCollabUser{sc})
			b, err := json.Marshal(col)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[{
        "id": 10,
        "user_id": 29,
        "container_id": 28,
        "created_at": "2013-11-01T00:00:00Z",
        "updated_at": "2013-11-02T00:00:00Z",
        "username": "mark",
        "name": "Marky Mark",
        "gravatar_hash": "0b66d7eea76eca3623b6e5fe4f05536d"
      }]`))
		})
	})
})
