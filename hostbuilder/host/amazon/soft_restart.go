package amazon

import (
	"fmt"
	"time"

	"golang.org/x/net/context"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
)

// SoftRestart sshs into host and stop-starts its freighter service
func (h *Host) SoftRestart() (*host.Info, error) {
	label := h.MetricsLabel("softrestart")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), h.createTimeout())
	defer cancel()

	hctx := host.NewContext(ctx, h, h, nil)
	defer func() {
		if hctx.Executor != nil {
			hctx.Executor.Close()
		}
	}()

	var steps = []host.Step{
		{
			Description: "Connecting to workspace",
			Func:        host.WaitForSSH,
			Timeout:     5 * time.Minute,
		},
		{
			Description: "Restarting freighter",
			Func:        host.RestartServices,
		},
	}

	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			errorMsg := fmt.Sprintf("ERROR: Failed in soft restart host step %q: %v", step.Description, err)
			h.Printf(errorMsg)
			h.logFailed()
			inf, _ := h.Info()
			return inf, err
		}
	}

	return h.Info()
}
