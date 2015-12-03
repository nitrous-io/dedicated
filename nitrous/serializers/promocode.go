package serializers

import (
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
)

type PromoCode struct {
	Slug         string `json:"slug"`
	TrialSeconds int64  `json:"trial_seconds,omitempty"`
}

func SerializePromoCode(pc *promocode.PromoCode) *PromoCode {
	if pc == nil {
		return nil
	}
	return &PromoCode{
		Slug:         pc.Slug,
		TrialSeconds: pc.TrialSeconds.Int64,
	}
}
