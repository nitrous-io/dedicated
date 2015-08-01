package serializers_test

import (
	"encoding/json"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("org serializer", func() {
	var (
		createdAt = time.Date(2013, time.November, 1, 0, 0, 0, 0, time.UTC)
		updatedAt = time.Date(2013, time.November, 2, 0, 0, 0, 0, time.UTC)

		model *org.Org
	)

	BeforeEach(func() {
		model = &org.Org{
			ID:        10,
			Name:      "Macrosoft Corp.",
			Slug:      "macrosoft",
			Email:     "bill@macrosoft.com",
			CreatedAt: &createdAt,
			UpdatedAt: &updatedAt,
		}
	})

	Describe("SerializeOrg", func() {
		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeOrg(model))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 10,
				"name": "Macrosoft Corp.",
				"slug": "macrosoft",
				"email": "bill@macrosoft.com",
				"created_at": "2013-11-01T00:00:00Z",
				"updated_at": "2013-11-02T00:00:00Z"
			}`))
		})
	})

	Describe("SerializeOrgs", func() {
		var model2 *org.Org

		BeforeEach(func() {
			model2 = &org.Org{
				ID:        11,
				Slug:      "foobar",
				Name:      "Foo Bar, Inc.",
				Email:     "eng@foobar.com",
				CreatedAt: &createdAt,
				UpdatedAt: &updatedAt,
			}
		})

		It("serializes to JSON", func() {
			b, err := json.Marshal(serializers.SerializeOrgs([]*org.Org{model, model2}))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`[
				{
					"id": 10,
					"name": "Macrosoft Corp.",
					"slug": "macrosoft",
					"email": "bill@macrosoft.com",
					"created_at": "2013-11-01T00:00:00Z",
					"updated_at": "2013-11-02T00:00:00Z"
				},
				{
					"id": 11,
					"name": "Foo Bar, Inc.",
					"slug": "foobar",
					"email": "eng@foobar.com",
					"created_at": "2013-11-01T00:00:00Z",
					"updated_at": "2013-11-02T00:00:00Z"
				}
			]`))
		})
	})
})
