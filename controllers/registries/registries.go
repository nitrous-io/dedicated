package registries

import (
	"net/http"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/registry"
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
	errAlreadyLoggedIn  = httperror.New(http.StatusBadRequest, "login_failed", "already logged in to this registry")
)

var (
	Index  middleware.Middleware = middleware.HostViaPath("slug", index)
	Login  middleware.Middleware = middleware.HostViaPath("slug", login)
	Logout middleware.Middleware = middleware.HostViaPath("slug", logout)
)

func init() {
	FreighterClient = freighterclient.NewClient(nil)
}

func index(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	registries, err := registry.FindAllByUserIDAndHostID(db, env.User.ID, h.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"registries": registries,
	}
	return nil
}

func login(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
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

	URL := r.FormValue("registry")

	// set the default registry URL to "hub.docker.com",
	// if user hasn't specified one.
	// This is only to record in the DB.
	if registryURL == "" {
		URL = "index.docker.io"
	}

	existingReg, err := registry.FindByUserIDAndHostIDAndURL(db, env.User.ID, h.ID, URL)
	if err != nil {
		return err
	}

	if existingReg != nil {
		return errAlreadyLoggedIn
	}

	params := &freighterclient.RegistryAuth{
		Username: username,
		Password: password,
		Email:    email,
		Registry: URL,
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

	reg := &registry.Registry{
		UserID:   env.User.ID,
		HostID:   h.ID,
		URL:      URL,
		Username: username,
		Email:    email,
	}

	err = reg.Insert(db)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
	logger.Infof("Response: 200 OK")

	return nil
}

func logout(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	logger := c.Logger()

	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}

	URL := r.FormValue("registry")

	err = FreighterClient.RegistryLogout(fHost, URL)
	if err != nil {
		if err == freighterclient.ErrInvalidParams {
			return errLogoutFailed
		}
		return err
	}

	if URL == "" {
		URL = "hub.docker.com"
	}

	err = registry.Delete(db, env.User.ID, h.ID, URL)
	if err != nil {
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
