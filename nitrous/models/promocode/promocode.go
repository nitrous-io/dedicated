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

type PromoCodeWithPlans struct {
	*PromoCode
	PlanIDs []int64
}

func (pp *PromoCodeWithPlans) AllowedPlan(pID int64) bool {
	for _, id := range pp.PlanIDs {
		if id == pID {
			return true
		}
	}
	return false
}

type PromoCodeUse struct {
	ID             int64         `db:"id"`
	PromoCodeID    int64         `db:"promo_code_id"`
	UserID         int64         `db:"user_id"`
	SubscriptionID sql.NullInt64 `db:"subscription_id"`
	CreatedAt      pq.NullTime   `db:"created_at"`
	UpdatedAt      pq.NullTime   `db:"updated_at"`
}

type PendingClaim struct {
	Use  *PromoCodeUse
	Code *PromoCode
}

func Find(d db.QueryReceiver, slug string) (*PromoCodeWithPlans, error) {
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
	var planIDs []int64
	err = d.Select(&planIDs, `SELECT plan_id FROM promo_code_plans WHERE promo_code_id = $1;`, pc.ID)
	if err != nil {
		return nil, err
	}
	return &PromoCodeWithPlans{
		PromoCode: pc,
		PlanIDs:   planIDs,
	}, nil
}

// allocates a claim for a promocode if (and only if) the plan provided
// is attached to that promo code slug
func Claim(d db.QueryReceiver, slug string, uID, pID int64) (*PromoCodeUse, error) {
	use := &PromoCodeUse{}
	err := d.Get(use, `INSERT INTO promo_code_uses(promo_code_id, user_id) SELECT promo_codes.id, $1 FROM (
			SELECT promo_codes.id, COUNT(uses.id) count FROM promo_codes
			LEFT JOIN promo_code_uses uses ON uses.promo_code_id = promo_codes.id
			WHERE disabled IS FALSE
			AND promo_codes.slug = $2
			AND (expires_at IS NULL OR expires_at > now())
			GROUP BY promo_codes.id
		) t
		INNER JOIN promo_codes ON t.id = promo_codes.id
		INNER JOIN promo_code_plans ON promo_code_plans.promo_code_id = promo_codes.id
		WHERE promo_code_plans.plan_id = $3
		AND t.count < promo_codes.quantity
		RETURNING *;`,
		uID, slug, pID)
	if err != nil {
		return nil, err
	}
	return use, nil
}

func FindPendingClaim(d db.QueryReceiver, uID, pID int64) (*PendingClaim, error) {
	use := &PromoCodeUse{}
	err := d.Get(use, `SELECT promo_code_uses.* FROM promo_code_uses
		INNER JOIN promo_code_plans ON promo_code_plans.promo_code_id = promo_code_uses.promo_code_id
		WHERE promo_code_plans.plan_id = $1
		AND subscription_id IS NULL AND user_id = $2;`, pID, uID)
	if err != nil {
		return nil, err
	}
	code := &PromoCode{}
	err = d.Get(code, `SELECT * FROM promo_codes WHERE id = $1;`, use.PromoCodeID)
	if err != nil {
		return nil, err
	}
	return &PendingClaim{
		Use:  use,
		Code: code,
	}, nil
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
