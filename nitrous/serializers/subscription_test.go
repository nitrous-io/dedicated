package serializers_test

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("subscription serializer", func() {
	var (
		timestamp = time.Date(2014, time.November, 1, 0, 0, 0, 0, time.UTC)
		model     *subscription.SubscriptionWithHostSlug
	)

	BeforeEach(func() {
		model = &subscription.SubscriptionWithHostSlug{
			Subscription: &subscription.Subscription{
				Slug:               "xcv4xa",
				ID:                 1,
				BillingProfileID:   2,
				PlanID:             3,
				State:              "activate",
				CurrentPeriodStart: pq.NullTime{time.Date(2015, time.June, 1, 0, 0, 0, 0, time.UTC), true},
				CurrentPeriodEnd:   pq.NullTime{time.Date(2015, time.July, 1, 0, 0, 0, 0, time.UTC), true},
				TrialEnd:           pq.NullTime{time.Date(2015, time.July, 1, 0, 0, 0, 0, time.UTC), true},
				CreatedAt:          &timestamp,
				UpdatedAt:          &timestamp,
				MultiHost:          true,
			},
		}
	})

	Describe("SerializeSubscription", func() {
		It("serializes to JSON", func() {
			b := serializers.SerializeSubscription(model)
			o, err := json.Marshal(b)
			Expect(err).To(BeNil())
			Expect(o).To(MatchJSON(`{
				"slug": "xcv4xa",
				"billing_profile_id": 2,
				"plan_id": 3,
				"state": "activate",
				"current_period_start": "2015-06-01T00:00:00Z",
				"current_period_end": "2015-07-01T00:00:00Z",
				"trial_end": "2015-07-01T00:00:00Z",
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z",
				"cancel_pending": false,
				"multi_host": true
			}`))
		})

		It("includes the host slug if present", func() {
			model.CancelPending = true
			model.HostSlug = sql.NullString{"abc", true}
			b := serializers.SerializeSubscription(model)
			o, err := json.Marshal(b)
			Expect(err).To(BeNil())
			Expect(o).To(MatchJSON(`{
				"slug": "xcv4xa",
				"billing_profile_id": 2,
				"plan_id": 3,
				"state": "activate",
				"current_period_start": "2015-06-01T00:00:00Z",
				"current_period_end": "2015-07-01T00:00:00Z",
				"trial_end": "2015-07-01T00:00:00Z",
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z",
				"cancel_pending": true,
				"host_slug": "abc",
				"multi_host": true
			}`))
		})

		It("includes the org ID if present", func() {
			model.OrgID = sql.NullInt64{23, true}
			b := serializers.SerializeSubscription(model)
			o, err := json.Marshal(b)
			Expect(err).To(BeNil())
			Expect(o).To(MatchJSON(`{
				"slug": "xcv4xa",
				"billing_profile_id": 2,
				"plan_id": 3,
				"state": "activate",
				"current_period_start": "2015-06-01T00:00:00Z",
				"current_period_end": "2015-07-01T00:00:00Z",
				"trial_end": "2015-07-01T00:00:00Z",
				"created_at": "2014-11-01T00:00:00Z",
				"updated_at": "2014-11-01T00:00:00Z",
				"cancel_pending": false,
				"org_id": 23,
				"multi_host": true
			}`))
		})
	})
})
