package coupon

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"strings"

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

// map of prefixes to Stripe coupon code
var mappings = map[string]string{
	"LHD": "lite.50.month",
	"LQD": "lite.25.month",
	"LMD": "lite.100.month",
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

// for coupons where we have a many codes -> 1 stripe coupon mapping
// if no mapping is found, return `Code` field
// this code can be deprecated past Aug 2015 (once coupons have expired)
func (c *Coupon) CodeForStripe() string {
	// for a Lite migration code to be valid, it must have the format
	// "<prefix>.<username>.<md5>", where <md5> is the first 4 characters of
	// the md5sum of <username>
	parts := strings.Split(c.Code, ".")
	if len(parts) != 3 {
		return c.Code
	}
	prefix, username, expected := parts[0], parts[1], parts[2]
	if _, ok := mappings[prefix]; !ok {
		return c.Code
	}

	md5s := md5.Sum([]byte(username))
	md5sum := hex.EncodeToString(md5s[:])
	if md5sum[:4] != expected {
		return c.Code
	}
	return mappings[prefix]
}
