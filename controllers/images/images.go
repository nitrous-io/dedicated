package images

import (
	"net/http"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
)

var (
	Index middleware.Middleware = middleware.HostViaPath("slug", index)
)

func index(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	crt, err := cert.Get(c.DB(), h.ID)
	if err != nil {
		return err
	}

	cl, err := mkFreighterClient(h, crt)
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

var mkFreighterClient = func(h *host.Host, c *cert.Cert) (freighterclient.Client, error) {
	if h.IPAddress.String == "" {
		return nil, controllers.ErrHostIPUnknown
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
