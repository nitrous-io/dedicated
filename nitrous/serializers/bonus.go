package serializers

import (
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
)

type Bonus struct {
	Slug   string `json:"slug"`
	Amount int64  `json:"amount"`
}

func SerializeBonus(bn *bonus.Bonus) *Bonus {
	if bn == nil {
		return nil
	}
	return &Bonus{
		Slug:   bn.Slug,
		Amount: bn.Amount,
	}
}

func SerializeBonuses(bns []*bonus.Bonus) []*Bonus {
	ret := make([]*Bonus, len(bns))
	for i, bn := range bns {
		ret[i] = SerializeBonus(bn)
	}
	return ret
}
