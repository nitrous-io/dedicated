package serializers_test

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/baseimage"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("base image serializer", func() {
	Describe("SerializeBaseImage", func() {
		now := time.Now()
		var model = &baseimage.BaseImage{
			ID:          5,
			Name:        "Go",
			Slug:        "golang",
			Repo:        "nitrousio/golang:latest",
			Description: "Golang image",
			CreatedAt:   &now,
			UpdatedAt:   &now,
		}

		It("serializes to JSON", func() {
			nowJSON, err := now.MarshalJSON()
			Expect(err).To(BeNil())

			r := serializers.SerializeBaseImage(model)
			b, err := json.Marshal(r)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
				"id": 5,
				"name": "Go",
				"slug": "golang",
				"repo": "nitrousio/golang:latest",
				"description": "Golang image",
				"created_at": %s,
				"updated_at": %s
			}`, nowJSON, nowJSON)))
		})
	})
})
