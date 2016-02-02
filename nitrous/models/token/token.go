package token

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

const (
	MaxLastUsedDuration = (24 * 7) * time.Hour
	ExpiryInSeconds     = MaxLastUsedDuration / time.Second
)

const (
	ScopeSeparator = " "

	// Default password login can have full access to user's account
	ScopeFull = "full"

	// Third party client have limited access
	// FIXME: Should check token scope when accessing
	// user settings like change password.
	ScopeThirdParty = "third_party"

	// Admin is impersonating a user, the same as ScopeFull but owned by an admin
	ScopeAdmin = "admin"

	TypeLogin          = "login"
	TypeDockerRegistry = "docker_registry"
)

var (
	// ErrInvalidToken is returned when the token is not found or expired
	ErrInvalidToken = errors.New("auth token is invalid")
)

type Token struct {
	ID         int64          `db:"id"`
	UserID     int64          `db:"user_id"`
	Token      string         `db:"token"`
	Type       string         `db:"type"`
	Scope      string         `db:"scope"`
	Nonce      sql.NullString `db:"nonce"`
	ClientID   sql.NullInt64  `db:"client_id"`
	LastIP     sql.NullString `db:"last_ip"`
	CreatedAt  *time.Time     `db:"created_at"`
	LastSeenAt *time.Time     `db:"last_seen_at"`
}

func (t *Token) Scopes() []string {
	return strings.Split(t.Scope, ScopeSeparator)
}

func (t *Token) HasScope(targetScope string) bool {
	for _, scope := range t.Scopes() {
		if scope == targetScope {
			return true
		}
	}
	return false
}

// token contains all of the target scopes
func (t *Token) HasAllScopes(targetScopes []string) bool {
	for _, targetScope := range targetScopes {
		if !t.HasScope(targetScope) {
			return false
		}
	}
	return true
}

// token contains at least one of the target scopes
func (t *Token) HasAtLeastScope(targetScopes []string) bool {
	for _, scope := range t.Scopes() {
		for _, targetScope := range targetScopes {
			if scope == targetScope {
				return true
			}
		}
	}

	return false
}

func FindByUser(d db.QueryReceiver, userID int64) (*Token, error) {
	t := &Token{}
	err := d.Get(t, `SELECT * FROM tokens WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1;`, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	if !t.IsValid() {
		return nil, ErrInvalidToken
	}
	return t, nil
}

func FindByToken(d db.QueryReceiver, token string) (*Token, error) {
	t := &Token{}
	err := d.Get(t, "SELECT * FROM tokens WHERE token = $1", token)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	if !t.IsValid() {
		return nil, ErrInvalidToken
	}
	return t, nil
}

func FindByType(d db.QueryReceiver, userID int64, typ string) (*Token, error) {
	t := &Token{}
	err := d.Get(t, `SELECT * FROM tokens
		WHERE user_id = $1 AND type = $2
		ORDER BY created_at DESC;`,
		userID, typ)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func DeleteExpiredTokens(d db.QueryReceiver) error {
	stmt := fmt.Sprintf(`DELETE FROM tokens WHERE last_seen_at <= now() - interval '%d milliseconds';`, MaxLastUsedDuration/time.Millisecond)
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

	// FIXME: default to full scope until we force
	// all tokens created must have scope specified
	if t.Scope == "" {
		t.Scope = ScopeFull
	}

	nstmt, err := d.PrepareNamed(`INSERT INTO tokens
		(user_id, token, type, last_ip, scope, nonce, client_id) VALUES
		(:user_id, :token, :type, :last_ip, :scope, :nonce, :client_id)
		RETURNING *;`)
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

// isValid checks the token if it's expired. The valid duration is hardcoded to
// MaxLastUsedDuration now
func (t *Token) IsValid() bool {
	if t == nil {
		return false
	}

	// If the token is issued by third party application,
	// we don't check expiry date
	if t.ClientID.Valid {
		return true
	}

	maxLastUsed := t.LastSeenAt.Add(MaxLastUsedDuration)
	return time.Now().Before(maxLastUsed)
}
