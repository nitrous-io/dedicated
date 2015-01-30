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
		Slug:        "rails-stack",
		Name:        sql.NullString{"Rails Stack", true},
		Description: sql.NullString{"A Rails stack template", true},
		Config:      `{ "does": "not get serialized" }`,
		Image:       "ubuntu-bare",
		Type:        template.TypeStack,
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
				"slug": "rails-stack",
				"name": "Rails Stack",
				"description": "A Rails stack template",
				"type": "%s",
				"created_at": %s,
				"updated_at": %s,
				"image": "ubuntu-bare",
				"privileged": false
			}`, template.TypeStack, nowJSON, nowJSON)))
		})

		It("should set privileged if it includes privileged containers", func() {
			prevConf := model.Config
			model.Config = `
				{
					"containers": [
						{
							"privileged": false
						},
						{
							"privileged": true
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
				"slug": "rails-stack",
				"name": "Rails Stack",
				"description": "A Rails stack template",
				"type": "%s",
				"created_at": %s,
				"updated_at": %s,
				"image": "ubuntu-bare",
				"privileged": true
			}`, template.TypeStack, nowJSON, nowJSON)))
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
				"slug": "rails-stack",
				"name": "Rails Stack",
				"description": "A Rails stack template",
				"type": "%s",
				"created_at": %s,
				"updated_at": %s,
				"image": "ubuntu-bare",
				"privileged": false
			}]`, template.TypeStack, nowJSON, nowJSON)))
		})
	})
})
