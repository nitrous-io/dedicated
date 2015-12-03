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

func SerializePromoCodes(pcs []*promocode.PromoCodeWithPlans) []*PromoCode {
	ret := make([]*PromoCode, len(pcs))
	for i, pc := range pcs {
		ret[i] = SerializePromoCode(pc)
	}
	return ret
}
