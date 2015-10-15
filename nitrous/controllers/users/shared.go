package users

import (
	"database/sql"
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errClientNotFound = httperror.New(http.StatusNotFound, "client_not_found", "client not found")
)

func getOAuthClient(env *middleware.Env, r *http.Request) (*oauthclient.Client, error) {
	clientID := r.FormValue("client_id")
	if clientID == "" {
		return nil, nil
	}

	clientSecret := r.FormValue("client_secret")

	client, err := oauthclient.FindWithSecret(env.DB, clientID, clientSecret)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errClientNotFound
		}
		return nil, err
	}

	return client, nil
}
