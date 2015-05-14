package logintoken

import (
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/nitrous-common/msgverifier"
)

type LoginToken struct {
	Type      string     `json:"type"`
	UserID    int64      `json:"user_id"`
	CreatedAt *time.Time `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at"`
}

var DefaultValidityPeriod time.Duration = 1 * time.Minute
var Secret = envvars.SecretToken

var (
	ErrInvalidToken = errors.New("logintoken: invalid token")
	ErrTokenExpired = errors.New("logintoken: token expired")
)

func Generate(userID int64) (string, error) {
	createdAt := time.Now().UTC()
	expiresAt := createdAt.Add(DefaultValidityPeriod)
	return msgverifier.New(Secret).Generate(&LoginToken{
		Type:      "logintoken",
		UserID:    userID,
		CreatedAt: &createdAt,
		ExpiresAt: &expiresAt,
	})
}

// returns user id if token is valid
func Read(token string) (int64, error) {
	var t *LoginToken
	err := msgverifier.New(Secret).Verify(token, &t)
	if err != nil {
		return 0, ErrInvalidToken
	}
	if t.Type != "logintoken" {
		return 0, ErrInvalidToken
	}
	if t.ExpiresAt.Before(time.Now()) {
		return 0, ErrTokenExpired
	}
	return t.UserID, nil
}
