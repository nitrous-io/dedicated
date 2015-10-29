package passwords

import (
	"database/sql"
	"net/http"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errInvalidToken = httperror.New(http.StatusNotFound, "invalid_token", "The password reset link is invalid.")
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

	if err := u.SendPasswordResetEmail(db); err != nil {
		return err
	}

	respond(env)
	return nil
}

func ResetPassword(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	token := r.FormValue("reset_token")
	if token == "" {
		return errInvalidToken
	}

	u, err := user.FindByResetPasswordToken(env.DB, token)
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

	err = env.DB.WithTransaction(func(tx *sqlx.Tx) error {
		if err := u.Update(tx); err != nil {
			return err
		}

		// Clear the reset password token so that it cannot be used again.
		if err := u.ClearResetPasswordToken(tx); err != nil {
			return err
		}

		// Clear all other login tokens.
		_, err = tx.Exec(`DELETE FROM tokens WHERE user_id = $1;`, u.ID)
		if err != nil {
			return err
		}
		return nil
	})
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
