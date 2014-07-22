package registry

import (
	"net/http"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var FreighterClient freighterclient.Client

var (
	errHostIPUnknown    = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
	ErrHostNotFound     = httperror.New(http.StatusNotFound, "not_found", "host does not exist") // TODO: should be shared across controllers?
	ErrUsernameRequired = httperror.New(http.StatusBadRequest, "invalid_request", "username is required")
	ErrPasswordRequired = httperror.New(http.StatusBadRequest, "invalid_request", "password is required")
	ErrEmailRequired    = httperror.New(http.StatusBadRequest, "invalid_request", "email is required")
)

var (
	Login  middlewares.Middleware = withHost(login)
	Logout middlewares.Middleware = withHost(logout)
)

func init() {
	FreighterClient = freighterclient.NewClient(nil)
}

func login(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	logger := c.Logger()

	username := r.FormValue("username")
	if username == "" {
		return ErrUsernameRequired
	}

	password := r.FormValue("password")
	if password == "" {
		return ErrPasswordRequired
	}

	email := r.FormValue("email")
	if email == "" {
		return ErrEmailRequired
	}

	registry := r.FormValue("registry")

	params := &freighterclient.RegistryAuth{
		Username: username,
		Password: password,
		Email:    email,
		Registry: registry,
	}

	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}

	err = FreighterClient.RegistryLogin(fHost, params)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
	logger.Infof("Response: 200 OK")

	return nil
}

func logout(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	logger := c.Logger()

	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}

	err = FreighterClient.RegistryLogout(fHost, r.FormValue("registry"))
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
	logger.Infof("Response: 200 OK")

	return nil
}

func withHost(fn func(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) middlewares.Middleware {
	return func(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		hSlug := env.Vars["slug"]
		h, err := host.FindByUserAndSlug(db, env.User, hSlug)
		if err != nil {
			return err
		}
		if h == nil {
			return ErrHostNotFound
		}

		return fn(h, env, c, w, r)
	}
}

func mkFreighterHost(h *host.Host) (*freighterclient.Host, error) {
	if h.IPAddress.String == "" {
		return nil, errHostIPUnknown
	}
	return freighterclient.NewHost(h.IPAddress.String, int(h.FreighterdPort.Int64), false), nil
}
