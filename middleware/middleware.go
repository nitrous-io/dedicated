package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/token"
	"github.com/nitrous-io/nitrous/models/user"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errInvalidRequest = httperror.New(http.StatusBadRequest, "invalid_request", "auth token is required")
	errInvalidToken   = httperror.New(http.StatusUnauthorized, "invalid_token", "auth token is invalid")
	ErrHostNotFound   = httperror.New(http.StatusNotFound, "not_found", "host does not exist")
)

type Env struct {
	Root   string
	Logger *logger.Logger
	DB     *db.Database
	Status int
	User   *user.User
	Data   map[string]interface{}
}

type Middleware func(env *Env, w http.ResponseWriter, r *http.Request) error

func Auth(mw Middleware) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		db := env.DB

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
		return mw(env, w, r)
	}
}

type hostHandlerFn func(h *host.Host, env *Env, w http.ResponseWriter, r *http.Request) error

func HostViaForm(srcVar string, fn hostHandlerFn) Middleware {
	return withHost("form", srcVar, fn)
}

func HostViaPath(srcVar string, fn hostHandlerFn) Middleware {
	return withHost("path", srcVar, fn)
}

// withHost takes a src ("form" or "path") and the formvalue/pattern to look
// for to retrieve host slug.
func withHost(src string, srcVar string, fn hostHandlerFn) Middleware {
	return func(env *Env, w http.ResponseWriter, r *http.Request) error {
		var hSlug string
		var h *host.Host

		switch src {
		case "form":
			hSlug = r.FormValue(srcVar)
		case "path":
			hSlug = mux.Vars(r)[srcVar]
		default:
			return errors.New("Invalid host source: " + src)
		}

		hID, err := strconv.Atoi(hSlug)
		// If err is not nil, that means this is an ID
		if err != nil {
			h, err = host.FindByUserAndSlug(env.DB, env.User, hSlug)
		} else {
			h, err = host.FindByUserAndID(env.DB, env.User, int64(hID))
		}

		if err != nil {
			return err
		}
		if h == nil {
			return ErrHostNotFound
		}

		return fn(h, env, w, r)
	}
}
