package serializers_test

import (
	"encoding/json"

	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("bonus serializer", func() {
	var (
		model, model2 *bonus.Bonus
		bonuses       []*bonus.Bonus

		model3, model4 *bonus.BonusUse
		bonusUses      []*bonus.BonusUse
	)

	BeforeEach(func() {
		model = &bonus.Bonus{
			ID:     2,
			Slug:   "follow-twitter",
			Amount: 3,
		}

		model2 = &bonus.Bonus{
			ID:     3,
			Slug:   "tweet",
			Amount: 5,
		}

		model3 = &bonus.BonusUse{
			UserID:  3,
			BonusID: 5,
		}

		model4 = &bonus.BonusUse{
			UserID:  6,
			BonusID: 7,
		}

		bonuses = []*bonus.Bonus{model, model2}
		bonusUses = []*bonus.BonusUse{model3, model4}
	})

	Describe("SerializeBonus", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializeBonus(model)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"slug": "follow-twitter",
				"amount": 3
			}`))
		})
	})

	Describe("SerializeBonuses", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializeBonuses(bonuses)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[
			{
				"slug": "follow-twitter",
				"amount": 3
			},
	    {
				"slug": "tweet",
				"amount": 5 
			}

			]`))
		})
	})

	Describe("SerializeBonusUse", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializeBonusUse(model3)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"user_id": 3,
				"bonus_id": 5 
			}`))
		})
	})

	Describe("SerializeBonusUses", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializeBonusUses(bonusUses)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[
			{
				"user_id": 3,
				"bonus_id": 5
			},
	    {
				"user_id": 6,
				"bonus_id": 7 
			}
			]`))
		})
	})

})
