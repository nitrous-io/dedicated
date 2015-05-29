package authorizationcode

import (
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
	"github.com/nitrous-io/nitrous-common/msgverifier"
)

var (
	ValidityPeriod = 1 * time.Minute
	verifier       = msgverifier.New(envvars.SecretToken)
)

var (
	ErrInvalidToken = errors.New("authorizationcode: invalid token")
	ErrCodeExpired  = errors.New("authorizationcode: authorization code expired")
)

type Code struct {
	Type        string    `json:"type"`
	UserID      int64     `json:"user_id"`
	ClientID    string    `json:"client_id"`
	Nonce       string    `json:"nonce"`
	RedirectURI string    `json:"redirect_uri"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func Generate(userID int64, clientID, redirectURI string) (string, error) {
	nonce, err := securerandom.Base64(48)
	if err != nil {
		return "", err
	}

	createdAt := time.Now().UTC()
	return verifier.Generate(&Code{
		Type:        "authorization_code",
		UserID:      userID,
		ClientID:    clientID,
		Nonce:       nonce,
		RedirectURI: redirectURI,
		CreatedAt:   createdAt,
		ExpiresAt:   createdAt.Add(ValidityPeriod),
	})
}

func Read(signedCode string) (*Code, error) {
	code := &Code{}
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
