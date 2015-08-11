package amazon

import (
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
)

// Restart performs a stop, then start of the underlying EC2 instance,
// preserving its public IP address. This is a "soft restart" compared to
// calling Stop() and then Start(), since this guarantees that the underlying
// EC2 instance is never changed.
func (h *Host) Restart() (*host.Info, error) {
	label := h.MetricsLabel("restart")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	if err := h.stopInstance(); err != nil {
		return nil, mh.Error(err, label)
	}

	if err := h.startInstance(); err != nil {
		return nil, mh.Error(err, label)
	}

	return h.Info()
}
