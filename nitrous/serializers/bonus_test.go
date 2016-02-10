package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("bonus serializer", func() {
	var timestamp = time.Date(2014, time.November, 1, 0, 0, 0, 0, time.UTC)

	var (
		bn, bn2 *bonus.Bonus
		bns     []*bonus.Bonus

		ebn, ebn2 *bonus.EarnedBonus
		ebns      []*bonus.EarnedBonus
	)

	BeforeEach(func() {
		bn = &bonus.Bonus{
			ID:        1,
			Slug:      "follow-twitter",
			Amount:    3,
			CreatedAt: &timestamp,
			UpdatedAt: &timestamp,
		}

		bn2 = &bonus.Bonus{
			ID:        2,
			Slug:      "tweet",
			Amount:    5,
			CreatedAt: &timestamp,
			UpdatedAt: &timestamp,
		}

		ebn = &bonus.EarnedBonus{
			ID:        3,
			UserID:    3,
			BonusID:   5,
			CreatedAt: &timestamp,
			UpdatedAt: &timestamp,
		}

		ebn2 = &bonus.EarnedBonus{
			ID:        4,
			UserID:    6,
			BonusID:   7,
			CreatedAt: &timestamp,
			UpdatedAt: &timestamp,
		}

		bns = []*bonus.Bonus{bn, bn2}
		ebns = []*bonus.EarnedBonus{ebn, ebn2}
	})

	Describe("SerializeBonus", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializeBonus(bn)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 1,
				"slug": "follow-twitter",
				"amount": 3,
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z"
			}`))
		})
	})

	Describe("SerializeBonuses", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializeBonuses(bns)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[
			{
				"id": 1,
				"slug": "follow-twitter",
				"amount": 3,
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z"
			},
	    {
				"id": 2,
				"slug": "tweet",
				"amount": 5,
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z"
			}
			]`))
		})
	})

	Describe("SerializeEarnedBonus", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializeEarnedBonus(ebn)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 3,
				"user_id": 3,
				"bonus_id": 5,
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z"
			}`))
		})
	})

	Describe("SerializeEarnedBonuses", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializeEarnedBonuses(ebns)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[
			{
				"id": 3,
				"user_id": 3,
				"bonus_id": 5,
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z"
			},
	    {
				"id": 4,
				"user_id": 6,
				"bonus_id": 7,
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z"
			}
			]`))
		})
	})

})
