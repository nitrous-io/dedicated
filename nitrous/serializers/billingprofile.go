package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
)

type BillingProfile struct {
	LastFour  string     `json:"last_four"`
	CardType  string     `json:"card_type"`
	ExpMonth  uint8      `json:"exp_month"`
	ExpYear   uint16     `json:"exp_year"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeBillingProfile(b *billingprofile.BillingProfile) *BillingProfile {
	if b == nil {
		return nil
	}
	return &BillingProfile{
		LastFour:  b.LastFour,
		CardType:  b.CardType,
		ExpMonth:  b.ExpMonth,
		ExpYear:   b.ExpYear,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}
