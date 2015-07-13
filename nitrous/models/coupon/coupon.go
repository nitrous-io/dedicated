package coupon

import (
	"errors"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Coupon struct {
	ID        int64       `db:"id"`
	UserID    int64       `db:"user_id"`
	Code      string      `db:"code"`
	UsedAt    pq.NullTime `db:"used_at"`
	CreatedAt pq.NullTime `db:"created_at"`
	UpdatedAt pq.NullTime `db:"updated_at"`
}

var ErrCouponExists = errors.New("user already has an existing coupon")

func Find(d db.QueryReceiver, userID int64) (*Coupon, error) {
	cp := &Coupon{}
	err := d.Get(cp, `SELECT * FROM coupons WHERE user_id = $1 AND used_at IS NULL;`, userID)
	if err != nil {
		return nil, err
	}
	return cp, nil
}

func (c *Coupon) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO coupons
		(user_id, code)
		VALUES (:user_id, :code)
		RETURNING *;`)
	if err != nil {
		return err
	}
	if err := nstmt.QueryRowx(c).StructScan(c); err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" && e.Constraint == "index_coupons_on_user_id" {
			return ErrCouponExists
		}
		return err
	}
	return nil
}

func (c *Coupon) MarkAsUsed(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE coupons SET used_at = now() WHERE id = :id RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}
