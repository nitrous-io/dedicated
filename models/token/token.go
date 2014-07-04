package token

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/pkg/securerandom"
)

type Token struct {
	ID        int64      `db:"id"`
	UserID    int64      `db:"user_id"`
	Token     string     `db:"token"`
	CreatedAt *time.Time `db:"created_at"`
}

func FindByToken(d db.QueryReceiver, token string) (*Token, error) {
	t := &Token{}
	err := d.Get(t, "SELECT * FROM tokens WHERE token = $1", token)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (t *Token) Insert(d db.QueryReceiver) error {
	if t.Token == "" {
		tok, err := securerandom.Base64(191)
		if err != nil {
			return err
		}
		t.Token = tok[:255]
	}

	nstmt, err := d.PrepareNamed(`INSERT INTO tokens (user_id, token) VALUES (:user_id, :token) RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(t).StructScan(t)
}
