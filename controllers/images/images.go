package images

import (
	"net/http"
	"strconv"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

// FreighterClient is a shared freighterclient.Client for making requests to Freighter
// API hosts.
var FreighterClient freighterclient.Client

var (
	errHostIPUnknown = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
	errHostNotFound  = httperror.New(http.StatusNotFound, "not_found", "host does not exist")

	errBadShowAllParameter = httperror.New(http.StatusBadRequest, "invalid_request", "show_all is a malformed boolean")
)

var (
	Index middlewares.Middleware = withHost(index)
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
	if h.IPAddress.String == "" {
		return nil, errHostIPUnknown
	}
	return freighterclient.NewHost(h.IPAddress.String, int(h.FreighterdPort.Int64), false), nil
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
			return errHostNotFound
		}

		return fn(h, env, c, w, r)
	}
}
