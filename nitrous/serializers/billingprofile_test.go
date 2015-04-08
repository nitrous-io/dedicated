package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("billingprofile serializer", func() {
	timestamp := time.Date(2014, time.November, 1, 0, 0, 0, 0, time.UTC)
	freeTrialExpiresAt := time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC)

	var (
		model = &billingprofile.BillingProfile{
			ID:                 10,                             // Not serialized.
			UserID:             25,                             // Not serialized.
			StripeID:           sql.NullString{"sc_001", true}, // Not serialized.
			LastFour:           sql.NullString{"4242", true},
			CardType:           sql.NullString{"Visa", true},
			ExpMonth:           sql.NullInt64{10, true},
			ExpYear:            sql.NullInt64{2018, true},
			State:              billingprofile.StateNoCard,
			FreeTrialExpiresAt: pq.NullTime{freeTrialExpiresAt, true},
			CreatedAt:          pq.NullTime{timestamp, true},
			UpdatedAt:          pq.NullTime{timestamp, true},
		}
	)

	Describe("SerializeBillingprofile", func() {
		It("serializes to JSON", func() {
			b := serializers.SerializeBillingProfile(model)
			o, err := json.Marshal(b)
			Expect(err).To(BeNil())
			Expect(o).To(MatchJSON(`{
				"last_four": "4242",
				"card_type": "Visa",
				"exp_month": 10,
				"exp_year": 2018,
				"state": "no_card",
				"free_trial_expires_at": "2015-01-01T00:00:00Z",
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z",
				"card_required": true,
				"has_billing_issues": false
			}`))
		})
	})
})
