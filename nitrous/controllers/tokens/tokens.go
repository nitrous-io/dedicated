package tokens

import (
	"net/http"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/bcrypt"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
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
		return err
	}

	if u == nil {
		return errInvalidCred
	}

	if !bcrypt.Verify(u.EncryptedPassword, password) {
		if err := user.IncrementLoginAttempts(db, login); err != nil {
			return err
		}
		return errInvalidCred
	}

	// FIXME ADMINONLY remove once pro is open to non-admins again
	if !u.Admin {
		return mh.Error(httperror.New(http.StatusBadRequest, "beta_closed", "The current private beta phase has ended.  The next beta phase will open in Jan 2015."), "nitrous.signin")
	}

	if err := user.ResetLoginAttempts(db, login); err != nil {
		return err
	}

	err = env.Login(u, w)
	if err != nil {
		return err
	}

	env.User = u
	env.Tracker.CreateProfile(u, env.Logger, "")
	return nil
}
