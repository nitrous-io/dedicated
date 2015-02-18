package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
)

type BillingProfile struct {
	LastFour  string     `json:"last_four"`
	CardType  string     `json:"card_type"`
	ExpMonth  int64      `json:"exp_month"`
	ExpYear   int64      `json:"exp_year"`
	State     string     `json:"state"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`

	CardRequired     bool `json:"card_required"`
	HasBillingIssues bool `json:"has_billing_issues"`
}

func SerializeBillingProfile(b *billingprofile.BillingProfile) *BillingProfile {
	if b == nil {
		return nil
	}
	return &BillingProfile{
		LastFour:         b.LastFour.String,
		CardType:         b.CardType.String,
		ExpMonth:         b.ExpMonth.Int64,
		ExpYear:          b.ExpYear.Int64,
		State:            b.State,
		CreatedAt:        b.CreatedAt,
		UpdatedAt:        b.UpdatedAt,
		CardRequired:     b.IsCardRequired(),
		HasBillingIssues: b.HasBillingIssues(),
	}
}
