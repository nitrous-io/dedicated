package oauth

import (
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	"github.com/nitrous-io/nitrous-common/msgverifier"
)

var (
	ValidityPeriod = 1 * time.Minute
	verifier       *msgverifier.MsgVerifier
)

var (
	ErrInvalidToken = errors.New("logintoken: invalid token")
	ErrCodeExpired  = errors.New("logintoken: authorization code expired")
)

func init() {
	verifier = msgverifier.New(envvars.SecretToken)
}

type AuthorizationCode struct {
	Type        string    `json:"type"`
	UserID      int64     `json:"user_id"`
	ClientID    string    `json:"client_id"`
	Nonce       string    `json:"nonce"`
	RedirectURI string    `json:"redirect_uri"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func GenerateAuthorizationCode(userID int64, clientID, redirectURI string) (string, error) {
	nonce, err := securerandom.Base64(191)
	if err != nil {
		return "", err
	}

	createdAt := time.Now().UTC()

	code := AuthorizationCode{
		Type:        "authorization_code",
		UserID:      userID,
		ClientID:    clientID,
		Nonce:       nonce,
		RedirectURI: redirectURI,
		CreatedAt:   createdAt,
		ExpiresAt:   createdAt.Add(ValidityPeriod),
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
		return nil, ErrCodeExpired
	}

	return code, nil
}
