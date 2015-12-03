package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("promocode serializer", func() {
	var (
		model *promocode.PromoCode
	)

	BeforeEach(func() {
		model = &promocode.PromoCode{
			ID:           2,
			Slug:         "abc",
			Disabled:     true,
			Quantity:     200,
			TrialSeconds: sql.NullInt64{200, true},
			ExpiresAt:    pq.NullTime{time.Now(), true},
		}
	})

	Describe("SerializePromoCode", func() {
		It("serializes to JSON", func() {
			r := serializers.SerializePromoCode(model)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
        "slug": "abc",
        "trial_seconds": 200
      }`))
		})

		It("should omit empty TrialSeconds", func() {
			model.TrialSeconds = sql.NullInt64{Valid: false}
			r := serializers.SerializePromoCode(model)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"slug": "abc"
			}`))
		})
	})
})
