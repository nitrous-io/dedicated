package serializers

import (
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
)

type Bonus struct {
	Slug   string `json:"slug"`
	Amount int64  `json:"amount"`
}

type BonusUse struct {
	UserID  int64 `json:"user_id"`
	BonusID int64 `json:"bonus_id"`
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

func SerializeBonusUse(bnu *bonus.BonusUse) *BonusUse {
	if bnu == nil {
		return nil
	}
	return &BonusUse{
		UserID:  bnu.UserID,
		BonusID: bnu.BonusID,
	}
}

func SerializeBonusUses(bnus []*bonus.BonusUse) []*BonusUse {
	ret := make([]*BonusUse, len(bnus))
	for i, bnu := range bnus {
		ret[i] = SerializeBonusUse(bnu)
	}
	return ret
}
