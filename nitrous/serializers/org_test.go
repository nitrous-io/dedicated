package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("org serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model *org.Org
	)

	BeforeEach(func() {
		model = &org.Org{
			ID:        10,
			Name:      "Macrosoft Corp.",
			Slug:      "macrosoft",
			Email:     "bill@macrosoft.com",
			MaxHosts:  sql.NullInt64{2, true},
			UsedHosts: 1,
			Edu:       true,
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		}
	})

	Describe("SerializeOrg", func() {
		It("should omit an empty MaxHosts", func() {
			model.MaxHosts = sql.NullInt64{Valid: false}
			serializedOrg := serializers.SerializeOrg(model)
			b, err := json.Marshal(serializedOrg)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"name": "Macrosoft Corp.",
				"slug": "macrosoft",
				"email": "bill@macrosoft.com",
				"multi_host": false,
				"used_hosts": 1,
				"edu": true,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})

		It("should omit an empty UsedHosts", func() {
			model.UsedHosts = 0
			serializedOrg := serializers.SerializeOrg(model)
			b, err := json.Marshal(serializedOrg)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"name": "Macrosoft Corp.",
				"slug": "macrosoft",
				"email": "bill@macrosoft.com",
				"max_hosts": 2,
				"multi_host": false,
				"edu": true,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})

		It("serializes to JSON", func() {
			serializedOrg := serializers.SerializeOrg(model)
			b, err := json.Marshal(serializedOrg)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"name": "Macrosoft Corp.",
				"slug": "macrosoft",
				"email": "bill@macrosoft.com",
				"max_hosts": 2,
				"used_hosts": 1,
				"multi_host": false,
				"edu": true,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})

		Context("when billing profile is added", func() {
			var billingProfile *billingprofile.BillingProfile

			BeforeEach(func() {
				billingProfile = &billingprofile.BillingProfile{
					ID:        10,
					UserID:    25,
					OrgID:     sql.NullInt64{12, true},
					StripeID:  sql.NullString{"sc_001", true},
					LastFour:  sql.NullString{"4242", true},
					CardType:  sql.NullString{"Visa", true},
					ExpMonth:  sql.NullInt64{10, true},
					ExpYear:   sql.NullInt64{2018, true},
					State:     billingprofile.StateNoCard,
					CreatedAt: &createdAt,
					UpdatedAt: &updatedAt,
				}
			})

			It("serializes to JSON with billing profile embedded", func() {
				serializedOrg := serializers.SerializeOrg(model)
				serializedOrg.BillingProfile = serializers.SerializeBillingProfile(billingProfile)
				b, err := json.Marshal(serializedOrg)
				Expect(err).To(BeNil())
				Expect(b).To(MatchJSON(`{
				"id": 10,
				"name": "Macrosoft Corp.",
				"slug": "macrosoft",
				"email": "bill@macrosoft.com",
				"max_hosts": 2,
				"used_hosts": 1,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z",
				"multi_host": false,
				"edu": true,
				"billing_profile": {
					"id": 10,
					"user_id": 25,
					"org_id": 12,
					"last_four": "4242",
					"card_type": "Visa",
					"exp_month": 10,
					"exp_year": 2018,
					"state": "no_card",
					"trial_eligible": false,
					"created_at": "2013-11-01T00:00:00Z",
					"updated_at": "2013-11-02T00:00:00Z",
					"card_required": true,
					"has_billing_issues": false
				}
			}`))
			})
		})
	})

	Describe("SerializeOrgWithRole", func() {
		It("serializes to JSON", func() {
			orgWithRole := &org.OrgWithRole{Org: model, Role: "owner"}

			serializedOrg := serializers.SerializeOrgWithRole(orgWithRole)
			b, err := json.Marshal(serializedOrg)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"name": "Macrosoft Corp.",
				"slug": "macrosoft",
				"email": "bill@macrosoft.com",
				"max_hosts": 2,
				"used_hosts": 1,
				"role": "owner",
				"multi_host": false,
				"edu": true,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})
	})

	Describe("SerializeOrgsWithRoles", func() {
		var model2 *org.Org

		BeforeEach(func() {
			model2 = &org.Org{
				ID:        11,
				Slug:      "foobar",
				Name:      "Foo Bar, Inc.",
				Email:     "eng@foobar.com",
				MaxHosts:  sql.NullInt64{2, true},
				UsedHosts: 1,
				CreatedAt: &createdAt,
				UpdatedAt: &updatedAt,
			}
		})

		It("serializes to JSON", func() {
			or1 := &org.OrgWithRole{
				Org:  model,
				Role: "limited",
			}
			or2 := &org.OrgWithRole{
				Org:  model2,
				Role: "owner",
			}
			b, err := json.Marshal(serializers.SerializeOrgsWithRoles([]*org.OrgWithRole{or1, or2}))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[
				{
					"id": 10,
					"name": "Macrosoft Corp.",
					"slug": "macrosoft",
					"email": "bill@macrosoft.com",
				  "max_hosts": 2,
				  "used_hosts": 1,
					"role": "limited",
					"multi_host": false,
					"edu": true,
					"created_at": "2013-11-01T00:00:00Z",
					"updated_at": "2013-11-02T00:00:00Z"
				},
				{
					"id": 11,
					"name": "Foo Bar, Inc.",
					"slug": "foobar",
					"email": "eng@foobar.com",
				  "max_hosts": 2,
				  "used_hosts": 1,
					"role": "owner",
					"multi_host": false,
					"edu": false,
					"created_at": "2013-11-01T00:00:00Z",
					"updated_at": "2013-11-02T00:00:00Z"
				}
			]`))
		})
	})
})
