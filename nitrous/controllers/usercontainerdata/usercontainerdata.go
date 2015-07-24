package usercontainerdata

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	ucd "github.com/nitrous-io/dedicated/nitrous/models/usercontainerdata"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	SetData = containers.WithContainerAndHost(setData, containers.AllowCollab)

	errKeyRequired   = httperror.New(http.StatusBadRequest, "invalid_request", "`key` is required")
	errValueRequired = httperror.New(http.StatusBadRequest, "invalid_request", "`value` is required")
)

func setData(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	key := r.FormValue("key")
	if key == "" {
		return errKeyRequired
	}

	value := r.FormValue("value")
	if value == "" {
		return errValueRequired
	}

	err := ucd.SetData(env.DB, env.User.ID, cn.ID, key, value)
	if err != nil {
		return err
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}
