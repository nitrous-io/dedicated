package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("user serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &user.User{
			ID:                10,
			Name:              "Emma Watson",                 // Not serialized.
			EncryptedPassword: "!!11such!11!secure!!11wow!!", // Not serialized.
			Password:          "doge4evar",                   // Not serialized.
			Username:          "emmawatson",
			Email:             "h.granger@hogwarts.edu",
			Admin:             true,
			UnconfirmedEmail:  sql.NullString{"h.granger+nospam@hogwarts.edu", true},
			PromoCode:         sql.NullString{"hogwarts", true},
			Phone:             sql.NullString{"88884444", true},
			PhoneCountryCode:  sql.NullString{"65", true},
			PhoneAuthed:       false,
			CreatedAt:         &createdAt,
			UpdatedAt:         &updatedAt,
		}
	)

	Describe("SerializeUser", func() {
		It("serializes to JSON", func() {
			u := serializers.SerializeUser(model)
			b, err := json.Marshal(u)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"name": "Emma Watson",
				"username": "emmawatson",
				"email": "h.granger@hogwarts.edu",
				"admin": true,
				"unconfirmed_email": "h.granger+nospam@hogwarts.edu",
				"promo_code": "hogwarts",
				"phone": "88884444",
				"phone_country_code": "65",
				"phone_authed": false,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})

		It("should omit an empty UnconfirmedEmail", func() {
			model.UnconfirmedEmail = sql.NullString{}
			u := serializers.SerializeUser(model)
			b, err := json.Marshal(u)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"name": "Emma Watson",
				"username": "emmawatson",
				"email": "h.granger@hogwarts.edu",
				"admin": true,
				"promo_code": "hogwarts",
				"phone": "88884444",
				"phone_country_code": "65",
				"phone_authed": false,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})
	})

	Describe("SerializeUserWithProfiles", func() {
		It("serializes to JSON", func() {
			up := &userprofile.UserProfile{
				ID:        10,
				UserID:    10,
				MaxHosts:  2,
				Advanced:  false,
				CreatedAt: &createdAt,
				UpdatedAt: &updatedAt,
			}

			freeTrialExpiresAt := time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC)
			bp := &billingprofile.BillingProfile{
				ID:                 10,
				UserID:             10,
				State:              "paid",
				StripeID:           sql.NullString{"sk_0001", true},
				LastFour:           sql.NullString{"1234", true},
				CardType:           sql.NullString{"Visa", true},
				ExpMonth:           sql.NullInt64{4, true},
				ExpYear:            sql.NullInt64{2014, true},
				FreeTrialExpiresAt: pq.NullTime{freeTrialExpiresAt, true},
				CreatedAt:          pq.NullTime{createdAt, true},
				UpdatedAt:          pq.NullTime{updatedAt, true},
			}

			u := serializers.SerializeUserWithProfiles(model, up, bp)
			b, err := json.Marshal(u)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"name": "Emma Watson",
				"username": "emmawatson",
				"email": "h.granger@hogwarts.edu",
				"admin": true,
				"user_profile": {
					"max_hosts": 2,
					"advanced": false
				},
				"billing_profile": {
					"state": "paid",
					"last_four": "1234",
					"card_type": "Visa",
					"exp_month": 4,
					"exp_year": 2014,
					"free_trial_expires_at": "2015-01-01T00:00:00Z",
					"created_at": "2013-11-01T00:00:00Z",
					"updated_at": "2013-11-02T00:00:00Z",
					"card_required": false,
					"has_billing_issues": false
				},
				"promo_code": "hogwarts",
				"phone": "88884444",
				"phone_country_code": "65",
				"phone_authed": false,
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})
	})
})
