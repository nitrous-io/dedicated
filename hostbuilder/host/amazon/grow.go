package amazon

import (
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
)

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
