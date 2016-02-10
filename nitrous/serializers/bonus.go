package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
)

type Bonus struct {
	ID        int64      `json:"id"`
	Slug      string     `json:"slug"`
	Amount    int64      `json:"amount"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

type EarnedBonus struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"user_id"`
	BonusID   int64      `json:"bonus_id"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeBonus(bn *bonus.Bonus) *Bonus {
	if bn == nil {
		return nil
	}
	return &Bonus{
		ID:        bn.ID,
		Slug:      bn.Slug,
		Amount:    bn.Amount,
		CreatedAt: bn.CreatedAt,
		UpdatedAt: bn.UpdatedAt,
	}
}

func SerializeBonuses(bns []*bonus.Bonus) []*Bonus {
	ret := make([]*Bonus, len(bns))
	for i, bn := range bns {
		ret[i] = SerializeBonus(bn)
	}
	return ret
}

func SerializeEarnedBonus(ebn *bonus.EarnedBonus) *EarnedBonus {
	if ebn == nil {
		return nil
	}
	return &EarnedBonus{
		ID:        ebn.ID,
		UserID:    ebn.UserID,
		BonusID:   ebn.BonusID,
		CreatedAt: ebn.CreatedAt,
		UpdatedAt: ebn.UpdatedAt,
	}
}

func SerializeEarnedBonuses(ebns []*bonus.EarnedBonus) []*EarnedBonus {
	ret := make([]*EarnedBonus, len(ebns))
	for i, ebn := range ebns {
		ret[i] = SerializeEarnedBonus(ebn)
	}
	return ret
}
