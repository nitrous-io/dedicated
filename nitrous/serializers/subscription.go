package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
)

type Subscription struct {
	Slug             string `json:"slug"`
	PlanID           int64  `json:"plan_id"`
	BillingProfileID int64  `json:"billing_profile_id"`

	State string `json:"state"`

	CurrentPeriodStart *time.Time `json:"current_period_start"`
	CurrentPeriodEnd   *time.Time `json:"current_period_end"`
	CancelPending      bool       `json:"cancel_pending"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`

	HostSlug string `json:"host_slug,omitempty"`

	Plan *Plan `json:"plan,omitempty"`
}

func SerializeSubscription(s *subscription.SubscriptionWithHostSlug) *Subscription {
	serialized := &Subscription{
		Slug:             s.Slug,
		PlanID:           s.PlanID,
		State:            s.State,
		BillingProfileID: s.BillingProfileID,
		CreatedAt:        s.CreatedAt,
		UpdatedAt:        s.UpdatedAt,
		HostSlug:         s.HostSlug.String,
		CancelPending:    s.CancelPending,
	}

	if s.CurrentPeriodStart.Valid {
		serialized.CurrentPeriodStart = &(s.CurrentPeriodStart.Time)
	}

	if s.CurrentPeriodEnd.Valid {
		serialized.CurrentPeriodEnd = &(s.CurrentPeriodEnd.Time)
	}

	return serialized
}
