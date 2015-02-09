package token

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

const (
	TypeLogin           = "login"
	TypeHost            = "host"
	TypeProvisioner     = "provisioner"
	MaxLastUsedDuration = (24 * 7) * time.Hour
)

type Token struct {
	ID         int64          `db:"id"`
	UserID     int64          `db:"user_id"`
	Token      string         `db:"token"`
	Type       string         `db:"type"`
	LastIP     sql.NullString `db:"last_ip"`
	CreatedAt  *time.Time     `db:"created_at"`
	LastSeenAt *time.Time     `db:"last_seen_at"`
}

func FindByUser(d db.QueryReceiver, userID int64) (*Token, error) {
	t := &Token{}
	err := d.Get(t, "SELECT * FROM tokens WHERE user_id = $1 ORDER BY created_at DESC", userID)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func FindByToken(d db.QueryReceiver, token string) (*Token, error) {
	t := &Token{}
	err := d.Get(t, "SELECT * FROM tokens WHERE token = $1", token)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func DeleteExpiredTokens(d db.QueryReceiver) error {
	stmt := fmt.Sprintf(`DELETE FROM tokens WHERE type = '%s' AND last_seen_at <= now() - interval '%d milliseconds';`, TypeLogin, MaxLastUsedDuration/time.Millisecond)
	_, err := d.Exec(stmt)
	if err != nil {
		return err
	}
	return nil
}

func (t *Token) Insert(d db.QueryReceiver) error {
	if t.Token == "" {
		tok, err := securerandom.Base64(191)
		if err != nil {
			return err
		}
		t.Token = tok[:255]
	}

	nstmt, err := d.PrepareNamed(`INSERT INTO tokens (user_id, token, type, last_ip) VALUES (:user_id, :token, :type, :last_ip) RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(t).StructScan(t)
}

func (t *Token) Touch(d db.QueryReceiver, ip string) error {
	var err error
	if ip != "" {
		_, err = d.Exec(`UPDATE tokens SET last_seen_at = now(), last_ip = $1 WHERE id = $2;`, ip, t.ID)
	} else {
		_, err = d.Exec(`UPDATE tokens SET last_seen_at = now() WHERE id = $1;`, t.ID)
	}
	if err != nil {
		return err
	}
	return nil
}

func (t *Token) Delete(d db.QueryReceiver) error {
	_, err := d.Exec(`DELETE FROM tokens WHERE id = $1`, t.ID)
	return err
}

func (t *Token) IsValid() bool {
	if t == nil {
		return false
	}
	maxLastUsed := t.LastSeenAt.Add(MaxLastUsedDuration)
	if time.Now().After(maxLastUsed) {
		return false
	}
	return true
}
