package freighterclienttools

import (
	"net/http"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	ErrHostIPUnknown = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
)

func MakeFreighterHost(h *host.Host, secure bool) (*freighterclient.Host, error) {
	if h.IPAddress.String == "" {
		return nil, ErrHostIPUnknown
	}

	return freighterclient.NewHost(h.IPAddress.String, int(h.FreighterdPort.Int64), secure), nil
}
