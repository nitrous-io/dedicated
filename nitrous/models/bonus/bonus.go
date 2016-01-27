package bonus

import (
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Bonus struct {
	ID        int64       `db:"id"`
	Slug      string      `db:"slug"`
	Amount    int64       `db:"amount"`
	CreatedAt pq.NullTime `db:"created_at"`
	UpdatedAt pq.NullTime `db:"updated_at"`
}

type BonusUse struct {
	ID        int64       `db:"id"`
	BonusID   int64       `db:"bonus_id"`
	UserID    int64       `db:"user_id"`
	CreatedAt pq.NullTime `db:"created_at"`
	UpdatedAt pq.NullTime `db:"updated_at"`
}

func FindBySlug(d db.QueryReceiver, slug string) (*Bonus, error) {
	bn := &Bonus{}
	err := d.Get(bn, `SELECT * FROM bonus WHERE slug = $1;`,
		slug)
	if err != nil {
		return nil, err
	}
	return bn, nil
}

func (bnu *BonusUse) Add(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO bonus_uses (bonus_id, user_id) VALUES (:bonus_id, :user_id) 
	  RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(bnu).StructScan(bnu)
}
