package oauth

import (
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	"github.com/nitrous-io/nitrous-common/msgverifier"
)

const DefaultValidityPeriod = 1 * time.Minute

var verifier *msgverifier.MsgVerifier

var (
	ErrInvalidToken = errors.New("logintoken: invalid token")
	ErrTokenExpired = errors.New("logintoken: token expired")
)

func init() {
	// TODO: get secret from envvars
	verifier = msgverifier.New("secret")
}

type AuthorizationCode struct {
	Type      string    `json:"type"`
	UserID    int64     `json:"user_id"`
	Nonce     string    `json:"nonce"`
	ClientID  string    `json:"client_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

func GenerateAuthorizationCode(userID int64, clientID string) (string, error) {
	nonce, err := securerandom.Base64(191)
	if err != nil {
		return "", err
	}

	createdAt := time.Now().UTC()

	code := AuthorizationCode{
		Type:      "authorization_code",
		UserID:    userID,
		ClientID:  clientID,
		Nonce:     nonce,
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(DefaultValidityPeriod),
	}

	return verifier.Generate(code)
}

func ReadAuthorizationCode(signedCode string) (*AuthorizationCode, error) {
	code := &AuthorizationCode{}

	err := verifier.Verify(signedCode, code)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if code.Type != "authorization_code" {
		return nil, ErrInvalidToken
	}
	if code.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}

	return code, nil
}
