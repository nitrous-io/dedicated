package bonus

import (
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

var (
	ErrBonusAlreadyEarned = errors.New("you have already earned this bonus")
)

type Bonus struct {
	ID        int64      `db:"id"`
	Slug      string     `db:"slug"`
	Amount    int64      `db:"amount"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

type EarnedBonus struct {
	ID        int64      `db:"id"`
	BonusID   int64      `db:"bonus_id"`
	UserID    int64      `db:"user_id"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func FindBySlug(d db.QueryReceiver, slug string) (*Bonus, error) {
	bn := &Bonus{}
	err := d.Get(bn, `SELECT * FROM bonuses WHERE slug = $1;`,
		slug)
	if err != nil {
		return nil, err
	}
	return bn, nil
}

func FindAll(d db.QueryReceiver) ([]*Bonus, error) {
	bns := []*Bonus{}
	err := d.Select(&bns, `SELECT * FROM bonuses;`)
	return bns, err
}

func (ebn *EarnedBonus) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO earned_bonuses (bonus_id, user_id) VALUES (:bonus_id, :user_id) 
	  RETURNING *;`)
	if err != nil {
		return err
	}

	if err := nstmt.QueryRowx(ebn).StructScan(ebn); err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			return ErrBonusAlreadyEarned
		}
		return err
	}

	return nil
}

func FindEarnedByUser(d db.QueryReceiver, userID int64) ([]*EarnedBonus, error) {
	ebns := []*EarnedBonus{}
	err := d.Select(&ebns, `SELECT * FROM earned_bonuses WHERE user_id = $1;`, userID)
	return ebns, err
}
