package containers

import (
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	errHostNotFound = httperror.New(http.StatusNotFound, "not_found", "host does not exist")
)

func Index(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	containers, err := container.FindAllByHostID(db, h.ID)
	if err != nil {
		return err
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"containers": containers,
	}
	return nil
}

func WithHost(fn func(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) middlewares.Middleware {
	return func(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		u := env.User
		if u == nil {
			return errors.New("user is nil")
		}

		hName := mux.Vars(r)["host"]
		h, err := host.FindByUserAndName(db, u, hName)
		if err != nil {
			return err
		}
		if h == nil {
			return errHostNotFound
		}

		return fn(h, env, c, w, r)
	}
}
