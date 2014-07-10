package middlewares

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errInvalidRequest = httperror.New(http.StatusBadRequest, "invalid_request")
	errInvalidToken   = httperror.New(http.StatusUnauthorized, "invalid_token")
)

type Env struct {
	Status int
	User   *user.User
	Data   map[string]interface{}
	Vars   map[string]string
}

type Middleware func(env *Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error

func Auth(mw Middleware) Middleware {
	return func(env *Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		authHeader := r.Header.Get("Authorization")
		prefix := "Bearer "
		if len(authHeader) <= len(prefix) ||
			(!strings.HasPrefix(authHeader, prefix) &&
				!strings.HasPrefix(authHeader, strings.ToLower(prefix))) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_request"`)
			return errInvalidRequest
		}

		tok := authHeader[len(prefix):]
		t, err := token.FindByToken(db, tok)
		if err != nil {
			return err
		}
		if t == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			return errInvalidToken
		}

		u, err := user.FindByID(db, t.UserID)
		if err != nil {
			return err
		}
		if u == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="nitrous", error="invalid_token"`)
			return errInvalidToken
		}

		env.User = u
		return mw(env, c, w, r)
	}
}

func Vars(mw Middleware) Middleware {
	return func(env *Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		env.Vars = mux.Vars(r)
		return mw(env, c, w, r)
	}
}

func JSON(mw Middleware) Middleware {
	return func(env *Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		err := mw(env, c, w, r)
		if err != nil {
			return err
		}
		if env.Status == 0 || env.Data == nil {
			return errors.New("Status and/or Data not set")
		}
		c.RespondEncodeJSON(w, env.Status, env.Data)
		return nil
	}
}
