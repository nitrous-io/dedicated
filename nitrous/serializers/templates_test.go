package serializers_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/template"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("base image serializer", func() {
	now := time.Now()
	model := &template.Template{
		ID:          5,
		Slug:        "rails-starter",
		Name:        sql.NullString{"Rails Starter", true},
		Description: sql.NullString{"A Rails starter template", true},
		Config:      `{ "does": "not get serialized" }`,
		Type:        template.StarterType,
		CreatedAt:   &now,
		UpdatedAt:   &now,
	}

	Describe("SerializeTemplate", func() {
		It("serializes to JSON", func() {
			nowJSON, err := now.MarshalJSON()
			Expect(err).To(BeNil())

			r := serializers.SerializeTemplate(model)
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 5,
				"slug": "rails-starter",
				"name": "Rails Starter",
				"description": "A Rails starter template",
				"type": "%s",
				"created_at": %s,
				"updated_at": %s,
				"images": []
			}`, template.StarterType, nowJSON, nowJSON)))
		})

		It("should expose images if present in config", func() {
			prevConf := model.Config
			model.Config = `
				{
					"containers": [
						{
							"image": "img1"
						}
					]
				}
			`

			defer func() { model.Config = prevConf }()

			nowJSON, err := now.MarshalJSON()
			Expect(err).To(BeNil())

			r := serializers.SerializeTemplate(model)
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 5,
				"slug": "rails-starter",
				"name": "Rails Starter",
				"description": "A Rails starter template",
				"type": "%s",
				"created_at": %s,
				"updated_at": %s,
				"images": ["img1"]
			}`, template.StarterType, nowJSON, nowJSON)))
		})
	})

	Describe("SerializeTemplates", func() {
		It("serializes to JSON", func() {
			nowJSON, err := now.MarshalJSON()
			Expect(err).To(BeNil())

			b, err := json.Marshal(serializers.SerializeTemplates([]*template.Template{model}))
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`[{
				"id": 5,
				"slug": "rails-starter",
				"name": "Rails Starter",
				"description": "A Rails starter template",
				"type": "%s",
				"created_at": %s,
				"updated_at": %s,
				"images": []
			}]`, template.StarterType, nowJSON, nowJSON)))
		})
	})
})
