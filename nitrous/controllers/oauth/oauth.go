package oauth

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthclient"
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
	grantTypePassword          = "password"
	grantTypeLoginToken        = "login_token"
	grantTypeAuthorizationCode = "authorization_code"
)

var TimerFunc func(d time.Duration) *time.Timer

func init() {
	TimerFunc = time.NewTimer
}

func IssueToken(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	var u *user.User
	var err error

	switch r.FormValue("grant_type") {
	case grantTypeAuthorizationCode:
		// OAuth 2.0 authorization: allows OAuth apps to exchange an
		// authorization code for an access token. This is the step after a user
		// has authorized an app to act on their behalf.
		return TokenFromAuthorizationCode(env, w, r)
	case grantTypeLoginToken:
		// Login token: a login token can be retrieved from the
		// `GET /user/login_token` endpoint, and it can be exchanged for an
		// access token. This is helpful to login a user automatically, e.g.
		// apps with an "Open in Nitrous IDE" button.
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
	case grantTypePassword:
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
	default:
		return errGrantType
	}

	if u == nil {
		return errUnknown
	}

	if r.FormValue("client_id") != "" {
		oauthClient, err := getOauthClient(env, r)
		if err != nil {
			return err
		}
		if err := env.LoginWithOauthClient(u, w, oauthClient); err != nil {
			return err
		}
	} else {
		if err = env.Login(u, w); err != nil {
			return err
		}
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

	// clear github_state cookie used for
	// GitHub auth if present to prevent CSRF
	if _, err := r.Cookie("github_state"); err == nil {
		http.SetCookie(w, &http.Cookie{
			Name:   "github_state",
			Value:  "",
			MaxAge: -1,
		})
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func getOauthClient(env *middleware.Env, r *http.Request) (*oauthclient.Client, error) {
	var client *oauthclient.Client
	var err error

	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")

	if clientID != "" && clientSecret != "" {
		client, err = oauthclient.FindWithSecret(env.DB, clientID, clientSecret)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, errClientNotFound
			}
			return nil, err
		}
	} else {
		return nil, errClientNotFound
	}
	return client, nil
}
