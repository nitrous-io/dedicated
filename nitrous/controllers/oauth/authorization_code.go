package oauth

import (
	"bytes"
	"database/sql"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthclient"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/authorizationcode"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous-common/must"
)

var (
	errClientNotFound           = httperror.New(http.StatusNotFound, "invalid_request", "client not found")
	errInvalidClientID          = httperror.New(http.StatusForbidden, "access_denied", "client ID does not match authorization code")
	errUnauthorizedTokenScope   = httperror.New(http.StatusForbidden, "access_denied", "token scope not allowed to request new authorization code")
	errInvalidClientSecret      = httperror.New(http.StatusBadRequest, "access_denied", "client secret must not be empty")
	errInvalidGrantType         = httperror.New(http.StatusBadRequest, "invalid_request", `grant_type must be "authorization_code"`)
	errInvalidAuthorizationCode = httperror.New(http.StatusBadRequest, "invalid_request", "invalid authorization code")
	errInvalidRedirectURI       = httperror.New(http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
	errUnauthorizedClient       = httperror.New(http.StatusUnauthorized, "access_denied", "invalid client secret")
	errCodeExpired              = httperror.New(http.StatusUnauthorized, "access_denied", "authorization code has expired")
	errCodeReused               = httperror.New(http.StatusUnauthorized, "access_denied", "authorization code cannot be reused")

	TmplAuthorize = mustParseTemplate("oauth/authorize", "oauth_authorize.html.tmpl")
)

func GetAuthorize(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	responseType := r.FormValue("response_type")
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	scope := r.FormValue("scope")
	state := r.FormValue("state")

	client, err := oauthclient.FindByClientID(env.DB, clientID)
	if err != nil {
		if err == sql.ErrNoRows {
			return errClientNotFound
		}
		return err
	}

	if redirectURI == "" {
		redirectURI = client.RedirectURI
	}

	ru, err := checkRedirectURI(redirectURI, client)
	if err != nil {
		return err
	}

	if responseType != "code" {
		redirect(env, w, *ru, map[string]string{
			"error":             "unsupported_response_type",
			"error_description": "The authorization server does not support obtaining an authorization code using this method.",
			"state":             state,
		})
		return nil
	}

	html := &bytes.Buffer{}
	data := struct {
		ClientID          string
		ClientName        string
		ClientDescription string
		RedirectURI       string
		Scope             string
		State             string
	}{
		client.ClientID,
		client.Name,
		client.Description,
		redirectURI,
		scope,
		state,
	}

	err = TmplAuthorize.Execute(html, data)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(html.Len()))
	w.WriteHeader(http.StatusOK)
	_, err = html.WriteTo(w)
	return err
}

func PostAuthorize(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	u := env.User
	t := env.Token

	state := r.FormValue("state")
	action := r.FormValue("action")
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")

	client, err := oauthclient.FindByClientID(env.DB, clientID)
	if err != nil {
		if err == sql.ErrNoRows {
			return errClientNotFound
		}
		return err
	}

	if redirectURI == "" {
		redirectURI = client.RedirectURI
	}

	ru, err := checkRedirectURI(redirectURI, client)
	if err != nil {
		return err
	}

	// TODO: only certain tokens should be able to authorize new clients
	if t.Scope != token.ScopeFull {
		redirect(env, w, *ru, map[string]string{
			"error":             "access_denied",
			"error_description": "The access token used does not permit authorization of new clients",
			"state":             state,
		})
		return nil
	}

	if action != "approve" {
		redirect(env, w, *ru, map[string]string{
			"error":             "access_denied",
			"error_description": "The resource owner or authorization server denied the request.",
			"state":             state,
		})
		return nil
	}

	ruStr := ru.String()
	code, err := authorizationcode.Generate(u.ID, clientID, ruStr)
	if err != nil {
		redirect(env, w, *ru, map[string]string{
			"error":             "server_error",
			"error_description": "The server failed to generate authorization code.",
			"state":             state,
		})
		return nil
	}

	redirect(env, w, *ru, map[string]string{
		"code":  code,
		"state": state,
	})
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

func ShowClient(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	clientID := mux.Vars(r)["clientid"]
	client, err := oauthclient.FindByClientID(env.DB, clientID)
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

func checkRedirectURI(uriStr string, client *oauthclient.Client) (*url.URL, error) {
	cu, err := url.Parse(client.RedirectURI)
	if err != nil {
		return nil, err
	}

	if uriStr == client.RedirectURI {
		return cu, nil
	}

	iu, err := url.Parse(uriStr)
	if err != nil {
		return nil, errInvalidRedirectURI
	}

	if !iu.IsAbs() || iu.Fragment != "" || iu.Scheme != cu.Scheme || iu.Host != cu.Host || iu.Path != cu.Path {
		return nil, errInvalidRedirectURI
	}

	return iu, nil
}

func appendQueryString(u url.URL, queries map[string]string) url.URL {
	values := u.Query()
	for k, v := range queries {
		values.Add(k, v)
	}
	u.RawQuery = values.Encode()
	return u
}

func redirect(env *middleware.Env, w http.ResponseWriter, uri url.URL, params map[string]string) {
	uri = appendQueryString(uri, params)
	uris := uri.String()
	w.Header().Set("Location", uris)
	env.Status = http.StatusFound
	env.Data = map[string]interface{}{}
}

var mustParseTemplate = func(name, file string) *template.Template {
	var tmpl string
	if !envvars.Testing {
		tmpl = string(must.ByteArray(ioutil.ReadFile(path.Join(envvars.RootDir, "templates", file))))
	}
	return template.Must(template.New(name).Parse(tmpl))
}
