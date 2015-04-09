package passwords

import (
	"database/sql"
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
		if err == sql.ErrNoRows {
			// respond with 200 OK and exit
			respond(env)
			return nil
		}
		return err
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

	rToken := r.FormValue("reset_token")
	if rToken == "" {
		return errInvalidToken
	}
	u, err := user.FindByResetPasswordToken(db, rToken)
	if err != nil {
		if err == sql.ErrNoRows {
			return errInvalidToken
		}
		return err
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

	_, err = db.Exec(`DELETE FROM tokens WHERE user_id = $1;`, u.ID)
	if err != nil {
		return err
	}

	respond(env)
	return nil
}

func respond(env *middleware.Env) {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{"status": "ok"}
}
