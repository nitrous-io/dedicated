package amazon

import (
	"io"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"golang.org/x/net/context"
)

// Update sets properties of an existing host
func (h *Host) Update(w io.Writer) (*host.Info, error) {
	label := h.MetricsLabel("update")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()

	hctx := host.NewContext(ctx, h, h, w)
	steps := []host.Step{
		{
			Description: "Setting workspace's name",
			Func:        associateInstanceTags,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			h.Printf("ERROR: Failed in update host step: %v", err)
			h.logFailed()
			if err == context.DeadlineExceeded {
				return nil, ErrTimeout
			}
			return nil, err
		}
	}

	return h.Info()
}
