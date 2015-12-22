package serializers_test

import (
	"database/sql"
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
		var member *orgmembership.OrgMember

		Context("when user exists", func() {
			BeforeEach(func() {
				member = &orgmembership.OrgMember{
					OrgMembership: orgmembership.OrgMembership{
						ID:        1,
						OrgID:     2,
						UserID:    sql.NullInt64{3, true},
						Role:      "owner",
						CreatedAt: &createdAt,
						UpdatedAt: &updatedAt,
					},
					OrgSlug:  "foobar-inc",
					Username: sql.NullString{"foo", true},
					Name:     sql.NullString{"Mr. Foo", true},
					Email:    sql.NullString{"foo@example.com", true},
				}
			})

			Describe("SerializeOrgMember", func() {
				It("serializes to JSON", func() {
					b, err := json.Marshal(serializers.SerializeOrgMember(member))
					Expect(err).To(BeNil())
					Expect(b).To(MatchJSON(`{
					"id": 1,
					"org_id": 2,
					"org_slug": "foobar-inc",
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
				var member2 *orgmembership.OrgMember

				BeforeEach(func() {
					member2 = &orgmembership.OrgMember{
						OrgMembership: orgmembership.OrgMembership{
							ID:        2,
							OrgID:     3,
							UserID:    sql.NullInt64{4, true},
							Role:      "member",
							CreatedAt: &createdAt,
							UpdatedAt: &updatedAt,
						},
						OrgSlug:  "macrosoft-corp",
						Username: sql.NullString{"bar", true},
						Name:     sql.NullString{"Ms. Bar", true},
						Email:    sql.NullString{"bar@example.com", true},
					}
				})

				It("serializes to JSON", func() {
					b, err := json.Marshal(serializers.SerializeOrgMembers([]*orgmembership.OrgMember{member, member2}))
					Expect(err).To(BeNil())
					Expect(b).To(MatchJSON(`[
					{
					"id": 1,
					"org_id": 2,
					"org_slug": "foobar-inc",
					"user_id": 3,
						"username": "foo",
						"name": "Mr. Foo",
						"email": "foo@example.com",
						"role": "owner",
						"created_at": "2013-11-01T00:00:00Z",
						"updated_at": "2013-11-02T00:00:00Z"
					},
					{
						"id": 2,
						"org_id": 3,
						"org_slug": "macrosoft-corp",
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

			Context("when pending email invite", func() {
				BeforeEach(func() {
					member = &orgmembership.OrgMember{
						OrgMembership: orgmembership.OrgMembership{
							ID:          1,
							OrgID:       2,
							InviteEmail: sql.NullString{"abc@foobar.com", true},
							Role:        "member",
							CreatedAt:   &createdAt,
							UpdatedAt:   &updatedAt,
						},
					}
				})

				It("should include PendingEmail if InviteEmail is present", func() {
					member.InviteEmail = sql.NullString{"abc@foobar.com", true}

					m := serializers.SerializeOrgMembers([]*orgmembership.OrgMember{member})
					b, err := json.Marshal(m)
					Expect(err).To(BeNil())
					Expect(b).To(MatchJSON(`[
					{
					"id": 1,
					"org_id": 2,
					"role": "member",
					"pending_email": "abc@foobar.com",
					"created_at": "2013-11-01T00:00:00Z",
					"updated_at": "2013-11-02T00:00:00Z"
					}
				]`))
				})
			})
		})
	})
})
