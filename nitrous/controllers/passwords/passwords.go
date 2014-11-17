package passwords

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errInvalidToken = httperror.New(http.StatusNotFound, "invalid_token", "invalid request - make sure the password reset link is correct")
)

func SendResetPasswordToken(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	login := r.FormValue("login")
	if login == "" {
		// respond with 200 OK and exit
		respond(env)
		return nil
	}

	u, err := user.FindByUsernameOrEmail(db, login)
	if err != nil {
		return err
	}
	if u == nil {
		// respond with 200 OK and exit
		respond(env)
		return nil
	}

	err = u.SendPasswordResetEmail(db)
	if err != nil {
		return err
	}

	respond(env)
	return nil
}

func ResetPassword(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	token := r.FormValue("reset_token")
	if token == "" {
		return errInvalidToken
	}
	u, err := user.FindByResetPasswordToken(db, token)
	if err != nil {
		return err
	}
	if u == nil {
		return errInvalidToken
	}

	if password := r.FormValue("password"); password != "" {
		u.Password = password
	}

	if err := u.Validate(); err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	if err := u.Update(db); err != nil {
		return err
	}

	respond(env)
	return nil
}

func respond(env *middleware.Env) {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{"status": "ok"}
}
