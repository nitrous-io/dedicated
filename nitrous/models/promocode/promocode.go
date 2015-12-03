package promocode

import (
	"database/sql"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type PromoCode struct {
	ID           int64         `db:"id"`
	Slug         string        `db:"slug"`
	Disabled     bool          `db:"disabled"`
	Quantity     int64         `db:"quantity"`
	TrialSeconds sql.NullInt64 `db:"trial_seconds"`
	ExpiresAt    pq.NullTime   `db:"expires_at"`
	CreatedAt    pq.NullTime   `db:"created_at"`
	UpdatedAt    pq.NullTime   `db:"updated_at"`
}

type PromoCodeUse struct {
	ID               int64         `db:"id"`
	PromoCodeID      int64         `db:"promo_code_id"`
	BillingProfileID int64         `db:"billing_profile_id"`
	SubscriptionID   sql.NullInt64 `db:"subscription_id"`
	CreatedAt        pq.NullTime   `db:"created_at"`
	UpdatedAt        pq.NullTime   `db:"updated_at"`
}

func Find(d db.QueryReceiver, slug string) (*PromoCode, error) {
	pc := &PromoCode{}
	err := d.Get(pc, `SELECT promo_codes.* FROM (
			SELECT promo_codes.id, COUNT(uses.id) count FROM promo_codes
			LEFT JOIN promo_code_uses uses ON uses.promo_code_id = promo_codes.id
			WHERE disabled IS FALSE
			AND promo_codes.slug = $1
			AND (expires_at IS NULL OR expires_at > now())
			GROUP BY promo_codes.id
		) t
		INNER JOIN promo_codes ON t.id = promo_codes.id
		WHERE t.count < promo_codes.quantity;`,
		slug)
	if err != nil {
		return nil, err
	}
	return pc, nil
}

func Claim(d db.QueryReceiver, slug string, bpID int64) (*PromoCodeUse, error) {
	use := &PromoCodeUse{}
	err := d.Get(use, `INSERT INTO promo_code_uses(promo_code_id, billing_profile_id) SELECT promo_codes.id, $1 FROM (
			SELECT promo_codes.id, COUNT(uses.id) count FROM promo_codes
			LEFT JOIN promo_code_uses uses ON uses.promo_code_id = promo_codes.id
			WHERE disabled IS FALSE
			AND promo_codes.slug = $2
			AND (expires_at IS NULL OR expires_at > now())
			GROUP BY promo_codes.id
		) t
		INNER JOIN promo_codes ON t.id = promo_codes.id
		WHERE t.count < promo_codes.quantity
		RETURNING *;`,
		bpID, slug)
	if err != nil {
		return nil, err
	}
	return use, nil
}

func (pc *PromoCodeUse) UpdateSubscriptionId(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE promo_code_uses
		SET (subscription_id, updated_at)
		= (:subscription_id, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pc).StructScan(pc)
}

func (pc *PromoCodeUse) Delete(d db.QueryReceiver) error {
	_, err := d.Exec(`DELETE FROM promo_code_uses WHERE id = $1`, pc.ID)
	return err
}
