package serializers_test

import (
	"encoding/json"

	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("plan serializer", func() {
	Describe("SerializePlan", func() {
		var model = &plan.Plan{
			ID:            123,
			Slug:          "1gb",
			Name:          "Small",
			MaxContainers: 3,
			Price:         1000,
			BasePrice:     2000,
			CPU:           "2",
			Memory:        "1 GB",
			Storage:       "20 GB",
			MostPopular:   true,
		}

		It("serializes to JSON", func() {
			p := serializers.SerializePlan(model)
			b, err := json.Marshal(p)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 123,
				"slug": "1gb",
				"name": "Small",
				"max_containers": 3,
				"price": 1000,
				"base_price": 2000,
				"cpu": "2",
				"memory": "1 GB",
				"storage": "20 GB",
				"most_popular": true
			}`))
		})
	})
})
