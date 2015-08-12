package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("billingprofile serializer", func() {
	var timestamp = time.Date(2014, time.November, 1, 0, 0, 0, 0, time.UTC)

	var (
		model = &billingprofile.BillingProfile{
			ID:        10,
			UserID:    25,
			OrgID:     sql.NullInt64{12, true},
			StripeID:  sql.NullString{"sc_001", true}, // Not serialized.
			LastFour:  sql.NullString{"4242", true},
			CardType:  sql.NullString{"Visa", true},
			ExpMonth:  sql.NullInt64{10, true},
			ExpYear:   sql.NullInt64{2018, true},
			State:     billingprofile.StateNoCard,
			CreatedAt: &timestamp,
			UpdatedAt: &timestamp,
		}
	)

	Describe("SerializeBillingprofile", func() {
		It("serializes to JSON", func() {
			b := serializers.SerializeBillingProfile(model)
			o, err := json.Marshal(b)
			Expect(err).To(BeNil())
			Expect(o).To(MatchJSON(`{
				"id": 10,
				"user_id": 25,
				"org_id": 12,
				"last_four": "4242",
				"card_type": "Visa",
				"exp_month": 10,
				"exp_year": 2018,
				"state": "no_card",
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z",
				"card_required": true,
				"has_billing_issues": false
			}`))
		})

		Context("when org_id is null", func() {
			BeforeEach(func() {
				model.OrgID = sql.NullInt64{Valid: false}
			})

			It("serializes to JSON with org_id as null", func() {
				b := serializers.SerializeBillingProfile(model)
				o, err := json.Marshal(b)
				Expect(err).To(BeNil())
				Expect(o).To(MatchJSON(`{
					"id": 10,
					"user_id": 25,
					"org_id": null,
					"last_four": "4242",
					"card_type": "Visa",
					"exp_month": 10,
					"exp_year": 2018,
					"state": "no_card",
					"created_at": "2014-11-01T00:00:00Z",
					"updated_at": "2014-11-01T00:00:00Z",
					"card_required": true,
					"has_billing_issues": false
				}`))
			})
		})
	})
})
