package tokens

import (
	"net/http"

	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/token"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errGrantType   = httperror.New(http.StatusBadRequest, "unsupported_grant_type")
	errUsername    = httperror.New(http.StatusBadRequest, "invalid_request", "username is required")
	errPassword    = httperror.New(http.StatusBadRequest, "invalid_request", "password is required")
	errInvalidCred = httperror.New(http.StatusBadRequest, "invalid_grant", "incorrect username or password")
)

func Post(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	grantType := r.FormValue("grant_type")
	username := r.FormValue("username")
	password := r.FormValue("password")

	if grantType != "password" {
		return errGrantType
	}

	if username == "" {
		return errUsername
	}

	if password == "" {
		return errPassword
	}

	u, err := user.FindByUsername(db, username)
	if err != nil {
		return err
	}
	if u == nil {
		return errInvalidCred
	}

	if !bcrypt.Verify(u.EncryptedPassword, password) {
		return errInvalidCred
	}

	t := &token.Token{UserID: u.ID}

	err = t.Insert(db)
	if err != nil {
		return err
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"access_token": t.Token,
		"token_type":   "Bearer",
	}
	return nil
}
