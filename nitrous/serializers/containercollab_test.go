package serializers_test

import (
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

		sc = &containercollab.ContainerCollabUser{
			containercollab.ContainerCollab{
				ID:          10,
				UserID:      29,
				ContainerID: 28,
				CreatedAt:   &createdAt,
				UpdatedAt:   &updatedAt,
			},
			"mark",
			"callum@callumj.com",
			"Marky Mark",
		}
	)

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
