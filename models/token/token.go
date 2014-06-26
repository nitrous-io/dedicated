package token

import (
	"time"

	"github.com/nitrous-io/nitrous/db"
)

type Token struct {
	ID        int64      `db:"id"`
	UserID    int64      `db:"user_id"`
	Token     string     `db:"token"`
	CreatedAt *time.Time `db:"created_at"`
}

func (t *Token) Insert(d db.QueryReceiver) error {
	q := "INSERT INTO tokens (user_id, token) VALUES (:user_id, :token);"
	_, err := d.NamedExec(q, t)
	return err
}
