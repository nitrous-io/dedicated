package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("orgmembership serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)
	)
	Describe("OrgMember", func() {
		var model *orgmembership.OrgMember

		BeforeEach(func() {
			model = &orgmembership.OrgMember{
				OrgMembership: orgmembership.OrgMembership{
					ID:        1,
					OrgID:     2,
					UserID:    3,
					Role:      "owner",
					CreatedAt: &createdAt,
					UpdatedAt: &updatedAt,
				},
				Username: "foo",
				Name:     "Mr. Foo",
				Email:    "foo@example.com",
			}
		})

		Describe("SerializeOrgMember", func() {
			It("serializes to JSON", func() {
				b, err := json.Marshal(serializers.SerializeOrgMember(model))
				Expect(err).To(BeNil())
				Expect(b).To(MatchJSON(`{
					"user_id": 3,
					"username": "foo",
					"name": "Mr. Foo",
					"email": "foo@example.com",
					"role": "owner",
					"created_at": "2013-11-01T00:00:00Z",
					"updated_at": "2013-11-02T00:00:00Z"
				}`))
			})
		})

		Describe("SerializeOrgMembers", func() {
			var model2 *orgmembership.OrgMember

			BeforeEach(func() {
				model2 = &orgmembership.OrgMember{
					OrgMembership: orgmembership.OrgMembership{
						ID:        2,
						OrgID:     2,
						UserID:    4,
						Role:      "member",
						CreatedAt: &createdAt,
						UpdatedAt: &updatedAt,
					},
					Username: "bar",
					Name:     "Ms. Bar",
					Email:    "bar@example.com",
				}
			})

			It("serializes to JSON", func() {
				b, err := json.Marshal(serializers.SerializeOrgMembers([]*orgmembership.OrgMember{model, model2}))
				Expect(err).To(BeNil())
				Expect(b).To(MatchJSON(`[
					{
						"user_id": 3,
						"username": "foo",
						"name": "Mr. Foo",
						"email": "foo@example.com",
						"role": "owner",
						"created_at": "2013-11-01T00:00:00Z",
						"updated_at": "2013-11-02T00:00:00Z"
					},
					{
						"user_id": 4,
						"username": "bar",
						"name": "Ms. Bar",
						"email": "bar@example.com",
						"role": "member",
						"created_at": "2013-11-01T00:00:00Z",
						"updated_at": "2013-11-02T00:00:00Z"
					}
				]`))
			})
		})
	})
})
