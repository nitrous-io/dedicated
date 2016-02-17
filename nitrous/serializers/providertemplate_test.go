package serializers_test

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("provider template serializer", func() {
	Describe("SerializeProviderTemplate", func() {
		now := time.Now()

		var model = &providertemplate.ProviderTemplate{
			ID:        1,
			RegionID:  2,
			PlanID:    3,
			Name:      "do-512MB",
			Provider:  "digitalocean",
			CreatedAt: &now,
			UpdatedAt: &now,
		}

		It("serializes to JSON", func() {
			nowJSON, err := now.MarshalJSON()
			Expect(err).To(BeNil())

			r := serializers.SerializeProviderTemplate(model)
			b, err := json.Marshal(r)

			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(fmt.Sprintf(`{
        "id": 1,
        "region_id": 2,
        "plan_id": 3,
        "name": "do-512MB",
        "provider": "digitalocean",
        "config": {},
        "created_at": %s,
        "updated_at": %s
      }`, nowJSON, nowJSON)))
		})
	})
})
