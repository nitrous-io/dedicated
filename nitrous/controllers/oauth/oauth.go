package oauth

import (
	"database/sql"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/oauth"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errClientIDMismatch = httperror.New(http.StatusBadRequest, "client_id_mismatch", "client ID does not match authorization code")
)

func ShowClient(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	clientID := mux.Vars(r)["clientid"]
	client, err := oauth.FindClientByID(env.DB, clientID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"oauth_client": client,
	}

	return nil
}

func Authorize(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// Submitted from web app, user is authenticated as usual
	// from Authorization header. User must be logged in.

	u := env.User
	clientID := r.FormValue("client_id")

	client, err := oauth.FindClientByID(env.DB, clientID)
	if err != nil {
		return err
	}

	code, err := oauth.GenerateAuthorizationCode(u.ID, clientID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"authorization_code": code,
		"redirect_uri":       client.RedirectURI,
	}

	return nil
}

func AuthorizationCode(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// Submitted from third party client, authenticated
	// with custom client secret.
	db := env.DB

	signedCode := r.FormValue("authorization_code")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	code, err := oauth.ReadAuthorizationCode(signedCode)
	if err != nil {
		return err
	}

	client, err := oauth.FindClientWithSecret(db, clientID, clientSecret)
	if err != nil {
		return err
	}

	if client.ClientID != clientID {
		return errClientIDMismatch
	}

	userID := client.UserID

	u, err := user.FindByID(db, userID)
	if err != nil {
		return err
	}

	t := &token.Token{
		UserID:   u.ID,
		Nonce:    sql.NullString{code.Nonce, true},
		ClientID: sql.NullInt64{client.ID, true},
	}

	err = t.Insert(db)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"access_token": t.Token,
		"token_type":   "authorization_grant",
	}

	return nil
}
