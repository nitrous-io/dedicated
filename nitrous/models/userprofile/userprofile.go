package userprofile

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type UserProfile struct {
	ID       int64 `db:"id"`
	UserID   int64 `db:"user_id"`
	MaxHosts int64 `db:"max_hosts"`
	Advanced bool  `db:"advanced"`

	ActivationEmailSentAt        *time.Time `db:"activation_email_sent_at"`
	StarterCreditEmailSendCount  int        `db:"starter_credit_email_send_count"`
	StarterCreditEmailLastSentAt *time.Time `db:"starter_credit_email_last_sent_at"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func Create(tx *sqlx.Tx, userID int64) error {
	_, err := tx.Exec(`INSERT INTO user_profiles (user_id) VALUES ($1);`, userID)
	return err
}

func Find(d db.QueryReceiver, userID int64) (*UserProfile, error) {
	p := &UserProfile{}
	err := d.Get(p, `SELECT * FROM user_profiles WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *UserProfile) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE user_profiles
		SET (advanced, updated_at) = (:advanced, now())
		WHERE id = :id RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(p).StructScan(p)
}
