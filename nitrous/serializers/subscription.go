package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
)

type Subscription struct {
	ID               int64 `json:"id"`
	PlanID           int64 `json:"plan_id"`
	BillingProfileID int64 `json:"billing_profile_id"`

	State string `json:"state"`

	CurrentPeriodStart *time.Time `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeSubscription(s *subscription.Subscription) *Subscription {
	serialized := &Subscription{
		ID:               s.ID,
		PlanID:           s.PlanID,
		State:            s.State,
		BillingProfileID: s.BillingProfileID,
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
	}

	if s.CurrentPeriodStart.Valid {
		serialized.CurrentPeriodStart = &(s.CurrentPeriodStart.Time)
	}

	if s.CurrentPeriodEnd.Valid {
		serialized.CurrentPeriodEnd = &(s.CurrentPeriodEnd.Time)
	}

	return serialized
}
