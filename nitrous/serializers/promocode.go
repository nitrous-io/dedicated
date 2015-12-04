package serializers

import (
	"github.com/nitrous-io/dedicated/nitrous/models/promocode"
)

type PromoCode struct {
	Slug         string  `json:"slug"`
	TrialSeconds int64   `json:"trial_seconds,omitempty"`
	PlanIDs      []int64 `json:"plan_ids"`
}

func SerializePromoCode(pc *promocode.PromoCodeWithPlans) *PromoCode {
	if pc == nil {
		return nil
	}
	return &PromoCode{
		Slug:         pc.Slug,
		TrialSeconds: pc.TrialSeconds.Int64,
		PlanIDs:      pc.PlanIDs,
	}
}
