package userprofile

import (
	"time"

	"github.com/nitrous-io/nitrous/db"
)

type UserProfile struct {
	ID       int64 `db:"id"`
	UserID   int64 `db:"user_id"`
	MaxHosts int64 `db:"max_hosts"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func Create(d db.QueryReceiver, userID int64) error {
	_, err := d.Exec(`INSERT INTO user_profiles (user_id) VALUES ($1);`, userID)
	return err
}
