package images

import (
	"net/http"
	"strconv"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/freighterclienttools"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

// FreighterClient is a shared freighterclient.Client for making requests to Freighter
// API hosts.
var FreighterClient freighterclient.Client

var (
	errBadShowAllParameter = httperror.New(http.StatusBadRequest, "invalid_request", "show_all is a malformed boolean")
)

var (
	Index middlewares.Middleware = middlewares.HostViaPath("slug", index)
)

func init() {
	FreighterClient = freighterclient.NewClient(nil)
}

func index(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	showAllStr := r.FormValue("show_all")
	showAll := false
	if len(showAllStr) != 0 {
		var err error
		showAll, err = strconv.ParseBool(showAllStr)
		if err != nil {
			return errBadShowAllParameter
		}
	}

	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}

	images, err := FreighterClient.ListImages(fHost, showAll)
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
	return freighterclienttools.MakeFreighterHost(h, false)
}
