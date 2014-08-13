package serializers_test

import (
	"encoding/json"

	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("plan serializer", func() {
	Describe("SerializePlan", func() {
		var model = &plan.Plan{
			ID:            123,
			Name:          "micro",
			MaxContainers: 3, // Not serialized.
		}

		It("serializes to JSON", func() {
			p := serializers.SerializePlan(model)
			b, err := json.Marshal(p)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 123,
				"name": "micro"
			}`))
		})
	})
})
