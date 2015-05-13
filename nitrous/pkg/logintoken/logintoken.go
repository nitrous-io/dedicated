package logintoken

import (
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	"github.com/nitrous-io/nitrous-common/msgverifier"
)

type LoginToken struct {
	UserID    int64      `json:"user_id"`
	Nonce     string     `json:"nonce"`
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
	nonce, err := securerandom.Base64(12)
	if err != nil {
		return "", err
	}
	return msgverifier.New(Secret).Generate(&LoginToken{
		UserID:    userID,
		Nonce:     nonce,
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
	if t.ExpiresAt.Before(time.Now()) {
		return 0, ErrTokenExpired
	}
	return t.UserID, nil
}
