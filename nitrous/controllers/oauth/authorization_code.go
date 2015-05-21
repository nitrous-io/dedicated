package oauth

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthclient"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/authorizationcode"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errAccessDenied             = httperror.New(http.StatusNotFound, "access_denied", "the resource owner has denied the request")
	errClientNotFound           = httperror.New(http.StatusNotFound, "invalid_request", "client ID not found")
	errInvalidClientID          = httperror.New(http.StatusForbidden, "access_denied", "client ID does not match authorization code")
	errUnauthorizedTokenScope   = httperror.New(http.StatusForbidden, "access_denied", "token scope not allowed to request new authorization code")
	errInvalidClientSecret      = httperror.New(http.StatusBadRequest, "access_denied", "client secret must not be empty")
	errInvalidGrantType         = httperror.New(http.StatusBadRequest, "invalid_request", "grant_type must be 'authorization_code'")
	errInvalidAuthorizationCode = httperror.New(http.StatusBadRequest, "invalid_request", "invalid authoriazation code")
	errInvalidRedirectURI       = httperror.New(http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
	errInvalidResponseType      = httperror.New(http.StatusBadRequest, "unsupported_response_type", "response type must be 'code'")
	errUnauthorizedClient       = httperror.New(http.StatusUnauthorized, "access_denied", "invalid client secret")
	errCodeExpired              = httperror.New(http.StatusUnauthorized, "access_denied", "authorization code has expired")
	errCodeReused               = httperror.New(http.StatusUnauthorized, "access_denied", "authorization code cannot be reused")
)

func ShowClient(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	clientID := mux.Vars(r)["clientid"]
	client, err := oauthclient.FindByID(env.DB, clientID)
	if err != nil {
		if err == sql.ErrNoRows {
			return errClientNotFound
		}

		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"oauth_client": client,
	}

	return nil
}

// TODO: do error redirection instead of showing them plain
// Implement OAuth2 Authorization Code Grant
// Specs: https://tools.ietf.org/html/rfc6749#page-24

// POST /api/v0/oauth/authorize
// Submitted from web app, user is authenticated as usual
// from Authorization header. User must be logged in.
func Authorize(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	u := env.User
	t := env.Token

	if t.Scope != token.ScopeFull {
		// Third party token is not allowed to
		// authorize new third party
		return errUnauthorizedTokenScope
	}

	responseType := r.FormValue("response_type")
	if responseType != "code" {
		return errInvalidResponseType
	}

	state := r.FormValue("state")
	clientID := r.FormValue("client_id")

	client, err := oauthclient.FindByID(env.DB, clientID)
	if err != nil {
		if err == sql.ErrNoRows {
			return errClientNotFound
		}
		return err
	}

	baseRedirectURI, err := url.ParseRequestURI(client.RedirectURI)
	if err != nil {
		return err
	}

	inputRedirectURI := r.FormValue("redirect_uri")

	if inputRedirectURI != "" && inputRedirectURI != client.RedirectURI {
		inputURI, err := url.ParseRequestURI(inputRedirectURI)
		if err != nil {
			return errInvalidRedirectURI
		}

		if !inputURI.IsAbs() || inputURI.Fragment != "" {
			return errInvalidRedirectURI
		}

		if inputURI.Scheme != baseRedirectURI.Scheme {
			return errInvalidRedirectURI
		}

		if inputURI.Host != baseRedirectURI.Host {
			return errInvalidRedirectURI
		}

		if inputURI.Path != baseRedirectURI.Path {
			return errInvalidRedirectURI
		}

		baseRedirectURI = inputURI
	}

	action := r.FormValue("action")
	if action != "approve" {
		return errAccessDenied
	}

	// redirect value is what client expects
	redirectValue := baseRedirectURI.String()

	code, err := authorizationcode.Generate(u.ID, clientID, redirectValue)
	if err != nil {
		return err
	}

	// calculate actual HTTP redirection URL that include
	// the authorization code result
	baseRedirectURI.RawQuery += "&code=" + url.QueryEscape(code) +
		"&state=" + url.QueryEscape(state) +
		"&redirect_uri=" + url.QueryEscape(redirectValue)

	w.Header().Set("Location", baseRedirectURI.String())

	env.Status = http.StatusFound
	env.Data = map[string]interface{}{
		"code":         code,
		"redirect_uri": redirectValue,
		"state":        state,
	}

	return nil
}

// POST /api/v0/oauth/token, grant_type=authorization_code
// Submitted from third party client, authenticated
// with custom client secret.
func TokenFromAuthorizationCode(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	grantType := r.FormValue("grant_type")
	if grantType != "authorization_code" {
		return errInvalidGrantType
	}

	signedCode := r.FormValue("code")
	code, err := authorizationcode.Read(signedCode)
	if err != nil {
		switch err {
		case authorizationcode.ErrCodeExpired:
			return errCodeExpired
		case authorizationcode.ErrInvalidToken:
			return errInvalidAuthorizationCode
		default:
			return err
		}
	}

	clientID := r.FormValue("client_id")
	if clientID != code.ClientID {
		if err == sql.ErrNoRows {
			return errUnauthorizedClient
		}
		return errInvalidClientID
	}

	redirectURI := r.FormValue("redirect_uri")
	if redirectURI != code.RedirectURI {
		return errInvalidRedirectURI
	}

	clientSecret := r.FormValue("client_secret")
	if clientSecret == "" {
		return errInvalidClientSecret
	}

	client, err := oauthclient.FindWithSecret(db, clientID, clientSecret)
	if err != nil {
		if err == sql.ErrNoRows {
			return errUnauthorizedClient
		}
		return err
	}

	// Ignore requested scope.
	// FIXME: Allow third party to request for different scopes
	scope := token.ScopeThirdParty

	userID := code.UserID

	u, err := user.FindByID(db, userID)
	if err != nil {
		return err
	}

	t := &token.Token{
		UserID:   u.ID,
		Nonce:    sql.NullString{code.Nonce, true},
		ClientID: sql.NullInt64{client.ID, true},
		Type:     "authorization_code",
		Scope:    scope,
	}

	err = t.Insert(db)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			return errCodeReused
		}
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"access_token": t.Token,
		"token_type":   "Bearer",
		"scope":        scope,
		"expires_in":   token.ExpiryInSeconds,
	}

	return nil
}
