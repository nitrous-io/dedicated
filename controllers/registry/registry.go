package registry

import (
	"net/http"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var FreighterClient freighterclient.Client

var (
	errHostIPUnknown    = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
	errUsernameRequired = httperror.New(http.StatusBadRequest, "invalid_request", "username is required")
	errPasswordRequired = httperror.New(http.StatusBadRequest, "invalid_request", "password is required")
	errEmailRequired    = httperror.New(http.StatusBadRequest, "invalid_request", "email is required")
	errLoginFailed      = httperror.New(http.StatusBadRequest, "login_failed", "registry login failed")
	errLogoutFailed     = httperror.New(http.StatusBadRequest, "logout_failed", "registry logout failed")
)

var (
	Login  middleware.Middleware = middleware.HostViaPath("slug", login)
	Logout middleware.Middleware = middleware.HostViaPath("slug", logout)
)

func init() {
	FreighterClient = freighterclient.NewClient(nil)
}

func login(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	logger := c.Logger()

	username := r.FormValue("username")
	if username == "" {
		return errUsernameRequired
	}

	password := r.FormValue("password")
	if password == "" {
		return errPasswordRequired
	}

	email := r.FormValue("email")
	if email == "" {
		return errEmailRequired
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
		if err == freighterclient.ErrInvalidParams {
			return errLoginFailed
		}

		return err
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
	logger.Infof("Response: 200 OK")

	return nil
}

func logout(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	logger := c.Logger()

	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}

	err = FreighterClient.RegistryLogout(fHost, r.FormValue("registry"))
	if err != nil {
		if err == freighterclient.ErrInvalidParams {
			return errLogoutFailed
		}
		return err
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
	logger.Infof("Response: 200 OK")

	return nil
}

func mkFreighterHost(h *host.Host) (*freighterclient.Host, error) {
	if h.IPAddress.String == "" {
		return nil, errHostIPUnknown
	}
	return freighterclient.NewHost(h.IPAddress.String, int(h.FreighterdPort.Int64), false), nil
}
