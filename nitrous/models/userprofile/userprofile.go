package userprofile

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"golang.org/x/oauth2"
)

var (
	ErrTokenEmpty    = errors.New("token is empty")
	ErrTokenNotValid = errors.New("token not valid")
)

type UserProfile struct {
	ID       int64 `db:"id"`
	UserID   int64 `db:"user_id"`
	MaxHosts int64 `db:"max_hosts"`
	Advanced bool  `db:"advanced"`

	GitHubUserName       sql.NullString `db:"github_user_name"`
	GitHubOAuthTokenJson sql.NullString `db:"github_oauth_token_json"`
	GitHubUserScopesJson sql.NullString `db:"github_user_scopes_json"`

	ActivationEmailSentAt        *time.Time `db:"activation_email_sent_at"`
	StarterCreditEmailSendCount  int        `db:"starter_credit_email_send_count"`
	StarterCreditEmailLastSentAt *time.Time `db:"starter_credit_email_last_sent_at"`

	SubscribeNewsletter    bool `db:"subscribe_newsletter"`
	SubscribeAnnouncements bool `db:"subscribe_announcements"`

	GitName  sql.NullString `db:"git_name"`
	GitEmail sql.NullString `db:"git_email"`

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

func FindByGitHubUserName(d db.QueryReceiver, gitHubUserName string) (*UserProfile, error) {
	p := &UserProfile{}
	err := d.Get(p, `SELECT * FROM user_profiles WHERE github_user_name = $1`, gitHubUserName)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func ConnectedToGithub(d db.QueryReceiver, userID int64) (bool, error) {
	var connected bool
	err := d.Get(&connected, `SELECT EXISTS
	        (SELECT 1 FROM user_profiles
		WHERE user_id = $1
		AND github_user_name IS NOT NULL
	        AND github_oauth_token_json IS NOT NULL
	        AND github_user_scopes_json IS NOT NULL);`, userID)
	if err != nil {
		return false, err
	}
	return connected, nil
}

func (p *UserProfile) UpdateGithubInfo(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE user_profiles
		SET (github_user_name, github_oauth_token_json, github_user_scopes_json)
			= (:github_user_name, :github_oauth_token_json, :github_user_scopes_json)
		WHERE id = :id RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(p).StructScan(p)
}

func (p *UserProfile) UpdateGitConfig(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE user_profiles
		SET (git_name, git_email)
			= (:git_name, :git_email)
		WHERE id = :id RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(p).StructScan(p)
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

func (p *UserProfile) AuthorizedGitHubUserName() string {
	userName := p.GitHubUserName.String
	if userName == "" || !p.GitHubUserName.Valid {
		return ""
	}

	token, err := p.GetTokenIfValidFromUserProfile()
	if err != nil || !token.Valid() {
		return ""
	}
	return userName
}

func (p *UserProfile) CanDeleteGitHubPublicKeys() bool {
	if !p.GitHubUserScopesJson.Valid || p.GitHubUserScopesJson.String == "" {
		return false
	}

	var scopes []string
	err := json.Unmarshal([]byte(p.GitHubUserScopesJson.String), &scopes)
	if err != nil {
		logrus.Infof("Failed to parse github scopes for user %+v: %+v", p, err)
		return false
	}

	for _, scope := range scopes {
		if scope == "admin:public_key" {
			return true
		}
	}

	return false
}

func (p *UserProfile) GetTokenIfValidFromUserProfile() (*oauth2.Token, error) {
	if p.GitHubOAuthTokenJson.String == "" {
		return nil, ErrTokenEmpty
	}
	token, err := tokenFromJSON(p.GitHubOAuthTokenJson.String)
	if err != nil {
		return nil, err
	}

	// it's valid for token to expire and become invalid
	if !token.Valid() {
		return nil, ErrTokenNotValid
	}

	return token, nil
}

func tokenFromJSON(jsonStr string) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := json.Unmarshal([]byte(jsonStr), &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func GetTokenIfValid(d db.QueryReceiver, userID int64) (*oauth2.Token, error) {
	p, err := Find(d, userID)
	if err != nil {
		return nil, err
	}
	return p.GetTokenIfValidFromUserProfile()
}
