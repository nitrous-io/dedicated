package registries

import (
	"net/http"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/registry"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

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

	registryURL := r.FormValue("registry")

	// set the default registry URL to "hub.docker.com",
	// if user hasn't specified one.
	// This is only to record in the DB.
	if registryURL == "" {
		registryURL = "index.docker.io"
	}

	existingReg, err := registry.FindByUserIDAndHostIDAndURL(db, env.User.ID, h.ID, registryURL)
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
		Registry: registryURL,
	}

	crt, err := cert.Get(db, h.ID)
	if err != nil {
		return err
	}

	cl, err := mkFreighterClient(h, crt)
	if err != nil {
		return err
	}

	err = cl.RegistryLogin(params)
	if err != nil {
		if err == freighterclient.ErrInvalidParams {
			return errLoginFailed
		}

		return err
	}

	reg := &registry.Registry{
		UserID:   env.User.ID,
		HostID:   h.ID,
		URL:      registryURL,
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

	crt, err := cert.Get(db, h.ID)
	if err != nil {
		return err
	}

	cl, err := mkFreighterClient(h, crt)
	if err != nil {
		return err
	}

	registryURL := r.FormValue("registry")

	err = cl.RegistryLogout(registryURL)
	if err != nil {
		if err == freighterclient.ErrInvalidParams {
			return errLogoutFailed
		}
		return err
	}

	if registryURL == "" {
		registryURL = "hub.docker.com"
	}

	err = registry.Delete(db, env.User.ID, h.ID, registryURL)
	if err != nil {
		return err
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte{})
	logger.Infof("Response: 200 OK")

	return nil
}

var mkFreighterClient = func(h *host.Host, c *cert.Cert) (freighterclient.Client, error) {
	if h.IPAddress.String == "" {
		return nil, errHostIPUnknown
	}
	hst := freighterclient.NewHost(
		h.IPAddress.String,
		int(h.FreighterdPort.Int64),
		h.Hostname(),
		[]byte(c.CA),
		[]byte(c.ClientCert),
		[]byte(c.ClientKey),
	)
	return freighterclient.NewClient(hst)
}
