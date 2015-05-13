package serializers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("credittype serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model = &credittype.CreditType{
			ID:           27,
			Name:         "Credit1",
			Description:  sql.NullString{"I am a desc", true},
			Active:       true,
			DefaultValue: 1782,
			CreatedAt:    &createdAt,
			UpdatedAt:    &updatedAt,
		}
	)

	Describe("SerializeCreditType", func() {
		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeCreditType(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
	        "id": 27,
	        "name": "Credit1",
	        "description": "I am a desc",
	        "active": true,
	        "default_value": 1782,
	        "created_at": "2013-11-01T00:00:00Z",
	        "updated_at": "2013-11-02T00:00:00Z"
	      }`)))
		})

		It("should include Stock & Used if provided", func() {
			model.Stock = sql.NullInt64{0, true}
			model.Used = sql.NullInt64{200, true}
			b, err := json.Marshal(serializers.SerializeCreditType(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
          "id": 27,
          "name": "Credit1",
          "description": "I am a desc",
          "active": true,
          "default_value": 1782,
          "stock": 0,
          "used": 200,
          "created_at": "2013-11-01T00:00:00Z",
          "updated_at": "2013-11-02T00:00:00Z"
        }`)))
		})

		It("should include expiry date if provided", func() {
			t := time.Date(2017, time.November, 1, 0, 0, 0, 0, time.UTC)
			model.ExpiresAt = pq.NullTime{t, true}
			b, err := json.Marshal(serializers.SerializeCreditType(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
          "id": 27,
          "name": "Credit1",
          "description": "I am a desc",
          "active": true,
          "default_value": 1782,
          "stock": 0,
          "used": 200,
          "created_at": "2013-11-01T00:00:00Z",
          "updated_at": "2013-11-02T00:00:00Z",
          "expires_at": "2017-11-01T00:00:00Z"
        }`)))
		})
	})

	Describe("SerializeCreditTypes", func() {
		It("serializes to a JSON array", func() {
			ary := []*credittype.CreditType{model}
			b, err := json.Marshal(serializers.SerializeCreditTypes(ary))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`[{
          "id": 27,
          "name": "Credit1",
          "description": "I am a desc",
          "active": true,
          "default_value": 1782,
          "stock": 0,
          "used": 200,
          "created_at": "2013-11-01T00:00:00Z",
          "updated_at": "2013-11-02T00:00:00Z",
          "expires_at": "2017-11-01T00:00:00Z"
        }]`)))
		})
	})
})
