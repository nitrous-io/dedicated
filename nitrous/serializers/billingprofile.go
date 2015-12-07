package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
)

type BillingProfile struct {
	ID            int64      `json:"id"`
	UserID        int64      `json:"user_id"`
	OrgID         *int64     `json:"org_id"` // can be null
	LastFour      string     `json:"last_four"`
	CardType      string     `json:"card_type"`
	ExpMonth      int64      `json:"exp_month"`
	ExpYear       int64      `json:"exp_year"`
	State         string     `json:"state"`
	TrialEligible bool       `json:"trial_eligible"`
	CreatedAt     *time.Time `json:"created_at"`
	UpdatedAt     *time.Time `json:"updated_at"`

	CardRequired     bool `json:"card_required"`
	HasBillingIssues bool `json:"has_billing_issues"`
}

func SerializeBillingProfile(b *billingprofile.BillingProfile) *BillingProfile {
	if b == nil {
		return nil
	}
	serialized := &BillingProfile{
		ID:               b.ID,
		UserID:           b.UserID,
		LastFour:         b.LastFour.String,
		CardType:         b.CardType.String,
		ExpMonth:         b.ExpMonth.Int64,
		ExpYear:          b.ExpYear.Int64,
		State:            b.State,
		TrialEligible:    b.TrialEligible,
		CreatedAt:        b.CreatedAt,
		UpdatedAt:        b.UpdatedAt,
		CardRequired:     b.IsCardRequired(),
		HasBillingIssues: b.HasBillingIssues(),
	}
	if b.OrgID.Valid {
		serialized.OrgID = &b.OrgID.Int64
	} else {
		serialized.OrgID = nil
	}
	return serialized
}
