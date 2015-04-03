package tokens

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errGrantType   = httperror.New(http.StatusBadRequest, "unsupported_grant_type")
	errLogin       = httperror.New(http.StatusBadRequest, "invalid_request", "username or email is required")
	errPassword    = httperror.New(http.StatusBadRequest, "invalid_request", "password is required")
	errInvalidCred = httperror.New(http.StatusBadRequest, "invalid_grant", "incorrect username or password")
)

var TimerFunc func(d time.Duration) *time.Timer

func init() {
	TimerFunc = time.NewTimer
}

func Post(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	grantType := r.FormValue("grant_type")
	login := r.FormValue("username") // this can be either username or email
	password := r.FormValue("password")

	if grantType != "password" {
		return errGrantType
	}

	if login == "" {
		return errLogin
	}

	// delay the request based on the number of login attempts
	delay, err := user.GetDelay(env.DB, login)
	if err != nil {
		return err
	}
	<-TimerFunc(delay).C

	if password == "" {
		return errPassword
	}

	u, err := user.FindByUsernameOrEmail(db, login)
	if err != nil {
		if err == sql.ErrNoRows {
			return errInvalidCred
		}
		return err
	}

	if !bcrypt.Verify(u.EncryptedPassword, password) {
		if err := user.IncrementLoginAttempts(db, login); err != nil {
			return err
		}
		return errInvalidCred
	}

	if err := user.ResetLoginAttempts(db, login); err != nil {
		return err
	}

	err = env.Login(u, w)
	if err != nil {
		return err
	}

	env.User = u
	return nil
}

func Logout(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	tk := env.Token

	if err := tk.Delete(db); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}
