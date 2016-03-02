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
				ID:                123,
				HostingModePlanID: sql.NullInt64{2, true},
				Slug:              "1gb",
				Name:              "Small",
				Description:       "Small plan - 1GB",
				Price:             1000,
				BasePrice:         2000,
				CPU:               "2",
				Memory:            "1 GB",
				Storage:           "20 GB",
				Hidden:            false,
				AutoShutdown:      true,
				MaxContainers:     3,
				Org:               false,
				Edu:               true,
			}
		})

		It("serializes to JSON", func() {
			p := serializers.SerializePlan(model)
			b, err := json.Marshal(p)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 123,
				"hosting_mode_plan_id": 2,
				"slug": "1gb",
				"name": "Small",
				"description": "Small plan - 1GB",
				"price": 1000,
				"base_price": 2000,
				"hourly_price": 1489,
				"cpu": "2",
				"memory": "1 GB",
				"storage": "20 GB",
				"subscription": false,
				"hidden": false,
				"trial_period_days": 0,
				"auto_shutdown": true,
				"max_containers": 3,
				"org": false,
				"edu": true
			}`))
		})

		It("marks subscription as true if the plan has a StripePlanID", func() {
			model.StripePlanID = sql.NullString{"plan_2", true}
			p := serializers.SerializePlan(model)
			b, err := json.Marshal(p)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 123,
				"hosting_mode_plan_id": 2,
				"slug": "1gb",
				"name": "Small",
				"max_containers": 3,
				"description": "Small plan - 1GB",
				"price": 1000,
				"base_price": 2000,
				"hourly_price": 1489,
				"cpu": "2",
				"memory": "1 GB",
				"storage": "20 GB",
				"subscription": true,
				"auto_shutdown": true,
				"hidden": false,
				"trial_period_days": 0,
				"max_containers": 3,
				"org": false,
				"edu": true
			}`))
		})

		It("includes MaxHosts if it is defined", func() {
			model.MaxHosts = sql.NullInt64{12, true}
			p := serializers.SerializePlan(model)
			b, err := json.Marshal(p)
			Expect(err).To(BeNil())
			Expect(b).To(MatchJSON(`{
				"id": 123,
				"hosting_mode_plan_id": 2,
				"slug": "1gb",
				"name": "Small",
				"description": "Small plan - 1GB",
				"price": 1000,
				"base_price": 2000,
				"hourly_price": 1489,
				"cpu": "2",
				"memory": "1 GB",
				"storage": "20 GB",
				"subscription": false,
				"hidden": false,
				"trial_period_days": 0,
				"auto_shutdown": true,
				"max_containers": 3,
				"max_hosts": 12,
				"org": false,
				"edu": true
			}`))
		})
	})
})
