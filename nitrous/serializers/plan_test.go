package serializers_test

import (
	"database/sql"
	"encoding/json"

	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("plan serializer", func() {
	Describe("SerializePlan", func() {
		var model *plan.Plan

		BeforeEach(func() {
			model = &plan.Plan{
				ID:              123,
				BasePlanID:      sql.NullInt64{1, true},
				Slug:            "1gb",
				Name:            "Small",
				Description:     "Small plan - 1GB",
				Type:            plan.TypeUsageBased,
				Price:           1000,
				BasePrice:       2000,
				CPU:             "2",
				Memory:          "1 GB",
				Storage:         "20 GB",
				MostPopular:     true,
				Hidden:          false,
				AutoShutdown:    true,
				MaxContainers:   3,
				MaxSnapshots:    10,
				CleanPreview:    true,
				DesktopApp:      false,
				SSH:             true,
				CustomDomain:    false,
				CustomTemplates: true,
				Org:             false,
			}
		})

		It("serializes to JSON", func() {
			p := serializers.SerializePlan(model)
			b, err := json.Marshal(p)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 123,
				"base_plan_id": 1,
				"slug": "1gb",
				"name": "Small",
				"description": "Small plan - 1GB",
				"type": "usage-based",
				"price": 1000,
				"base_price": 2000,
				"hourly_price": 1489,
				"cpu": "2",
				"memory": "1 GB",
				"storage": "20 GB",
				"most_popular": true,
				"subscription": false,
				"hidden": false,
				"auto_shutdown": true,
				"max_containers": 3,
				"max_snapshots": 10,
				"clean_preview": true,
				"desktop_app": false,
				"ssh": true,
				"custom_domain": false,
				"custom_templates": true,
				"org": false
			}`))
		})

		It("marks subscription as true if the plan has a StripePlanID", func() {
			model.StripePlanID = sql.NullString{"plan_2", true}
			p := serializers.SerializePlan(model)
			b, err := json.Marshal(p)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 123,
				"base_plan_id": 1,
				"slug": "1gb",
				"name": "Small",
				"type": "usage-based",
				"max_containers": 3,
				"description": "Small plan - 1GB",
				"type": "usage-based",
				"price": 1000,
				"base_price": 2000,
				"hourly_price": 1489,
				"cpu": "2",
				"memory": "1 GB",
				"storage": "20 GB",
				"most_popular": true,
				"subscription": true,
				"auto_shutdown": true,
				"hidden": false,
				"max_containers": 3,
				"max_snapshots": 10,
				"clean_preview": true,
				"desktop_app": false,
				"ssh": true,
				"custom_domain": false,
				"custom_templates": true,
				"org": false
			}`))
		})

		It("omits base_plan_id if empty", func() {
			model.BasePlanID = sql.NullInt64{}
			p := serializers.SerializePlan(model)
			b, err := json.Marshal(p)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 123,
				"slug": "1gb",
				"name": "Small",
				"type": "usage-based",
				"max_containers": 3,
				"description": "Small plan - 1GB",
				"type": "usage-based",
				"price": 1000,
				"base_price": 2000,
				"hourly_price": 1489,
				"cpu": "2",
				"memory": "1 GB",
				"storage": "20 GB",
				"most_popular": true,
				"subscription": false,
				"hidden": false,
				"auto_shutdown": true,
				"max_containers": 3,
				"max_snapshots": 10,
				"clean_preview": true,
				"desktop_app": false,
				"ssh": true,
				"custom_domain": false,
				"custom_templates": true,
				"org": false
			}`))
		})
	})
})
