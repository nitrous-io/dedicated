package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
)

type BillingProfile struct {
	LastFour           string     `json:"last_four"`
	CardType           string     `json:"card_type"`
	ExpMonth           int64      `json:"exp_month"`
	ExpYear            int64      `json:"exp_year"`
	State              string     `json:"state"`
	FreeTrialExpiresAt *time.Time `json:"free_trial_expires_at"`
	CreatedAt          *time.Time `json:"created_at"`
	UpdatedAt          *time.Time `json:"updated_at"`

	CardRequired     bool `json:"card_required"`
	HasBillingIssues bool `json:"has_billing_issues"`
}

func SerializeBillingProfile(b *billingprofile.BillingProfile) *BillingProfile {
	if b == nil {
		return nil
	}
	serialized := &BillingProfile{
		LastFour:         b.LastFour.String,
		CardType:         b.CardType.String,
		ExpMonth:         b.ExpMonth.Int64,
		ExpYear:          b.ExpYear.Int64,
		State:            b.State,
		CardRequired:     b.IsCardRequired(),
		HasBillingIssues: b.HasBillingIssues(),
	}
	if b.FreeTrialExpiresAt.Valid {
		serialized.FreeTrialExpiresAt = &b.FreeTrialExpiresAt.Time
	}
	if b.CreatedAt.Valid {
		serialized.CreatedAt = &b.CreatedAt.Time
	}
	if b.UpdatedAt.Valid {
		serialized.UpdatedAt = &b.UpdatedAt.Time
	}

	return serialized
}
