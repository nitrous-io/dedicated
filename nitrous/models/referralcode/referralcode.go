package referralcode

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

type ReferralCode struct {
	ID     int64  `db:"id"`
	UserID int64  `db:"user_id"` // Referer's user id
	Code   string `db:"code"`

	Bonus int64 `db:"bonus"` // In seconds, to be added to usage_quota in users
	Cap   int64 `db:"cap"`   // Number of times this code can be used

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

type ReferralCodeUse struct {
	ID             int64      `db:"id"`
	ReferralCodeID int64      `db:"referral_code_id"`
	UserID         int64      `db:"user_id"` // Referee's user id
	CreatedAt      *time.Time `db:"created_at"`
	UpdatedAt      *time.Time `db:"updated_at"`
}

func Create(d db.QueryReceiver, userID int64) (*ReferralCode, error) {
	rc := &ReferralCode{}

	code, err := securerandom.Alphanumeric(20)
	if err != nil {
		return rc, err
	}

	err = d.QueryRowx(`INSERT INTO referral_codes
		(user_id, code)
		VALUES ($1, $2)
		RETURNING *;`, userID, code).StructScan(rc)
	if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
		return Create(d, userID)
	}
	return rc, err
}

func Use(d *db.Database, refCode string, refereeID int64) (*ReferralCodeUse, error) {
	rcu := &ReferralCodeUse{}
	err := d.QueryRowx(`INSERT INTO referral_code_uses
		(referral_code_id, user_id)
		SELECT id, $1
		FROM referral_codes AS rc
		WHERE (
			rc.code = $2
			AND (
				(SELECT COUNT(*) FROM referral_code_uses WHERE referral_code_id=rc.id) < rc.cap
			)
		)
		RETURNING *;`, refereeID, refCode).StructScan(rcu)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, capExceeded(d, refCode)
		}
		return nil, err
	}

	return rcu, grantBonuses(d, refereeID, rcu)
}

func capExceeded(d *db.Database, refCode string) error {
	bn, err := bonus.FindBySlug(d, "refer-friend")
	if err != nil {
		if err == sql.ErrNoRows {
			bn, err = bonus.Insert(d, "refer-friend", 108000)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	var refererID int64
	err = d.Get(&refererID, `SELECT user_id FROM referral_codes WHERE code=$1;`, refCode)
	if err != nil {
		return err
	}

	ebn := &bonus.EarnedBonus{
		BonusID: bn.ID,
		UserID:  refererID,
	}
	return ebn.Insert(d)
}

func grantBonuses(d *db.Database, refereeID int64, rcu *ReferralCodeUse) error {
	// Grant bonus to referee
	_, err := d.Exec(`UPDATE users
	  SET usage_quota = usage_quota + bonus,
		  updated_at = now()
		FROM (
			SELECT id, bonus
			FROM referral_codes
		) as rc
		WHERE (
			users.id = $1
			AND rc.id = $2
		)
		RETURNING *;`, refereeID, rcu.ReferralCodeID)
	if err != nil {
		return err
	}
	// Grant bonus to referer
	_, err = d.Exec(`UPDATE users
	  SET usage_quota = usage_quota + bonus,
		  updated_at = now()
		FROM (
			SELECT id, user_id, bonus
			FROM referral_codes
		) as rc
		WHERE (
			users.id = rc.user_id
			AND rc.id = $1
		)
		RETURNING *;`, rcu.ReferralCodeID)
	return err
}
