package serializers_test

import (
	"encoding/json"

	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("region serializer", func() {
	Describe("SerializeRegion", func() {
		var model = &region.Region{ID: 5, Slug: "apse", Name: "Southeast Asia"}

		It("serializes to JSON", func() {
			r := serializers.SerializeRegion(model)
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 5,
				"slug": "apse",
				"name": "Southeast Asia"
			}`))
		})
	})
})
