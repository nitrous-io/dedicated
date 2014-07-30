package images

import (
	"net/http"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/host"
)

// FreighterClient is a shared freighterclient.Client for making requests to Freighter
// API hosts.
var FreighterClient freighterclient.Client

var (
	Index middleware.Middleware = middleware.HostViaPath("slug", index)
)

func init() {
	FreighterClient = freighterclient.NewClient(nil)
}

func index(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}

	images, err := FreighterClient.ListImages(fHost)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"images": images,
	}

	return nil
}

func mkFreighterHost(h *host.Host) (*freighterclient.Host, error) {
	return controllers.MakeFreighterHost(h, false)
}
