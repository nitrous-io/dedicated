package oauthtoken

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

var (
	Twitter = "twitter"
)

type OAuthToken struct {
	ID           int64          `db:"id"`
	UserID       int64          `db:"user_id"`
	Service      string         `db:"service"` // eg. "twitter", "facebook", "github"
	NitrousToken sql.NullString `db:"nitrous_token"`
	// Store the request token in order to obtain
	// the access token in the oauth callback
	RequestToken sql.NullString `db:"request_token"`

	// AccessToken and Secret are used to perform
	// authenticated requests
	AccessToken sql.NullString `db:"access_token"`
	Secret      sql.NullString `db:"secret"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func (t *OAuthToken) Upsert(d db.QueryReceiver) error {
	_, err := d.Exec(`WITH upsert AS
        (UPDATE oauth_tokens
        SET request_token = $1, nitrous_token = $2, updated_at = now()
        WHERE user_id = $3 AND service = $4 RETURNING *)
        INSERT INTO oauth_tokens (user_id, service, request_token, nitrous_token)
        SELECT $3, $4, $1, $2 WHERE NOT EXISTS (SELECT * FROM upsert);`,
		t.RequestToken, t.NitrousToken, t.UserID, t.Service)
	return err
}

func (t *OAuthToken) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE oauth_tokens 
        SET (access_token, secret, updated_at)
        = (:access_token, :secret, now())
        WHERE request_token = :request_token
        RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(t).StructScan(t)
}

func FindByTokenAndService(d db.QueryReceiver, reqToken, service string) (*OAuthToken, error) {
	t := &OAuthToken{}
	err := d.Get(t, `SELECT * FROM oauth_tokens WHERE request_token = $1 AND service = $2;`,
		reqToken, service)
	return t, err
}

func FindByUserAndService(d db.QueryReceiver, userID int64, service string) (*OAuthToken, error) {
	t := &OAuthToken{}
	err := d.Get(t, `SELECT * FROM oauth_tokens WHERE user_id = $1 AND service = $2;`,
		userID, service)
	return t, err
}
