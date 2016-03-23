package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/referralcode"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("referralcode serializer", func() {
	var timestamp = time.Date(2014, time.November, 1, 0, 0, 0, 0, time.UTC)

	var rc *referralcode.ReferralCode

	BeforeEach(func() {
		rc = &referralcode.ReferralCode{
			ID:        1,
			UserID:    1,
			Code:      "abc123",
			Bonus:     999,
			Cap:       10,
			CreatedAt: &timestamp,
			UpdatedAt: &timestamp,
		}
	})

	Describe("SerializeReferralCode", func() {
		It("serializes to JSON", func() {
			s := serializers.SerializeReferralCode(rc)
			r, err := json.Marshal(s)

			Expect(err).To(BeNil())
			Expect(r).To(MatchJSON(`{
				"id": 1,
				"user_id": 1,
				"code": "abc123",
				"bonus": 999,
				"cap": 10,
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z"
			}`))
		})
	})

})
