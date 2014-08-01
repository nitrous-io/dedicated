package images

import (
	"net/http"

	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
)

var (
	Index middleware.Middleware = middleware.HostViaPath("slug", index)
)

func index(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(c.DB(), h)
	if err != nil {
		return err
	}

	images, err := cl.ListImages()
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"images": images,
	}

	return nil
}
