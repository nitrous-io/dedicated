package containers

import (
	"net/http"
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

var (
	RunCommand = WithContainerAndHost(runCommand, TouchHost)
)

func runCommand(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
	if err != nil {
		return err
	}

	timeout, err := strconv.ParseInt(r.FormValue("timeout"), 10, 64)
	if err != nil {
		return err
	}
	err = cl.RunCommand(cn.CID, r.FormValue("command"), timeout)
	if err != nil {
		return err
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}

	return nil
}
