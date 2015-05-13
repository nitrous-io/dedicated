package oauth

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

func ShowClient(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// Get client name and description from database

	// Return as json

	return nil
}

func Authorize(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// Submitted from web app, user is authenticated as usual
	// from Authorization header. User must be logged in.

	// Validate client ID and other parameters with
	// database oauth_clients table.

	// Generate authorization code with HMAC

	// Return code, redirection URL, etc as json.
	// web app will perform the browser redirection.

	return nil
}

func AuthorizationCode(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// Submitted from third party client, authenticated
	// with custom client secret.

	// Get client ID and secret from POST form

	// Validate with database

	// Get authorization code from POST form

	// Unpack and validate with HMAC

	// Generate and insert token into database

	// Return token response in json

	return nil
}
