package tokens

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/controllers/oauth"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/logintoken"
)

var (
	errGrantType   = httperror.New(http.StatusBadRequest, "unsupported_grant_type")
	errLogin       = httperror.New(http.StatusBadRequest, "invalid_request", "username or email is required")
	errPassword    = httperror.New(http.StatusBadRequest, "invalid_request", "password is required")
	errInvalidCred = httperror.New(http.StatusBadRequest, "invalid_grant", "incorrect username or password")

	errInvalidLoginToken = httperror.New(http.StatusBadRequest, "invalid_grant", "invalid login token")

	errUnknown = httperror.New(http.StatusInternalServerError, "unknown_error", "unknown auth error")
)

const (
	grantTypePassword   = "password"
	grantTypeLoginToken = "login_token"
)

var TimerFunc func(d time.Duration) *time.Timer

func init() {
	TimerFunc = time.NewTimer
}

func Post(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	grantType := r.FormValue("grant_type")

	var u *user.User
	var err error

	if grantType == grantTypePassword {
		login := r.FormValue("username") // this can be either username or email
		password := r.FormValue("password")

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

		u, err = user.FindByUsernameOrEmail(db, login)
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
	} else if grantType == grantTypeLoginToken {
		loginToken := r.FormValue("token")

		if loginToken == "" {
			return errInvalidLoginToken
		}

		uid, err := logintoken.Read(loginToken)
		if err != nil {
			return errInvalidLoginToken
		}

		u, err = user.FindByID(db, uid)
		if err != nil {
			if err == sql.ErrNoRows {
				return errInvalidLoginToken
			}
			return err
		}
	} else if grantType == "authorization_code" {
		return oauth.TokenFromAuthorizationCode(env, w, r)
	} else {
		return errGrantType
	}

	if u == nil {
		return errUnknown
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
