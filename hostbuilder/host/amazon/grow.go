package amazon

import (
	"time"

	"golang.org/x/net/context"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
)

// Grow grows the instance pool by a host whose parameters are defined by h
func (h *Host) Grow() (*host.Info, error) {
	label := h.MetricsLabel("grow")
	defer mh.UpdateSince(label, time.Now())

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

	for _, step := range h.growSteps() {
		if err := hctx.Run(step.Step); err != nil {
			h.Printf("ERROR: failed in step %q: %v", step.Description, err)

			if step.RecoveryFunc != nil {
				h.Printf("attempting to recover from grow failure in step %q", step.Step.Description)
				if err := step.RecoveryFunc(hctx); err != nil {
					h.Printf("failed to recover from grow failure, err: %v", err)
				}
			}

			if err == context.DeadlineExceeded {
				return nil, ErrTimeout
			}
			return nil, err
		}
	}

	return h.Info()
}

func (h *Host) growSteps() []host.StepWithRecovery {
	return []host.StepWithRecovery{
		{
			Step: host.Step{
				Description: "Checking image's availability",
				Func:        inspectSourceAMI,
			},
		},
		{
			Step: host.Step{
				Description: "Creating workspace",
				Func:        createInstance,
				Timeout:     5 * time.Minute,
			},
		},
		{
			Step: host.Step{
				Description: "Setting workspace's name",
				Func:        associateInstanceTags,
			},
		},
		{
			Step: host.Step{
				Func:    host.WaitForSSH,
				Timeout: 5 * time.Minute,
			},
		},
		{
			Step: host.Step{
				Description: "Assigning network address",
				Func:        attachElasticIP,
			},
			RecoveryFunc: func(ctx *host.HostContext) error {
				h := ctx.Host.(*Host)
				return h.recoverAttachElasticIP()
			},
		},
		{
			Step: host.Step{
				Description: "Connecting via SSH",
				Func:        host.WaitForSSH,
				Timeout:     5 * time.Minute,
			},
		},
		{
			Step: host.Step{
				Description: "Discovering new disk",
				Func: func(ctx *host.HostContext) error {
					// Get info of the volume attached to instance, so that we can
					// detach and delete it.
					h := ctx.Host.(*Host)

					userVolume, err := h.getVolumeByDeviceName(userDeviceName)
					if err != nil {
						return err
					}

					h.userVolume = userVolume

					return nil
				},
			},
		},
		{
			Step: host.Step{
				Description: "Unmount new volume",
				Func:        unmountUserVolume,
			},
		},
		{
			Step: host.Step{
				Description: "Detaching new volume",
				Func:        detachUserVolume,
			},
		},
		{
			Step: host.Step{
				Description: "Deleting new volume",
				Func:        deleteOldVolume,
			},
		},
	}
}
