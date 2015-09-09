package amazon

import (
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"golang.org/x/net/context"
)

// Start starts an instance and allocate the desired IP address to it
func (h *Host) Start() (*host.Info, error) {
	label := h.MetricsLabel("start")
	defer mh.UpdateSince(label, time.Now())

	// [Feature flag] - requires EBS hotswapping.
	if h.Config.Hotswap {
		h.Printf("starting hotswap instance")
		return h.startWithPool()
	}

	h.Printf("starting pre-EBS hotswapping instance normally")

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	if err := h.startInstance(); err != nil {
		return nil, mh.Error(err, label)
	}

	return h.Info()
}

func (h *Host) startWithPool() (*host.Info, error) {
	instanceType := h.Config.InstanceType

	h.Printf("attempting to acquire instance of type %q from pool", instanceType)

	// Attempt to acquire an instance from pool.
	instance, err := h.acquireInstance(h.provider, instanceType)
	if err != nil {
		h.Printf("could not acquire an instance from pool for instance type %q, err: %v", instanceType, err)

		// If we can't acquire an instance from the pool, we create a new
		// instance and attach the user's existing volume.
		return h.attachToNewInstance(h.Config.VolumeID)
	}

	label := h.MetricsLabel("start_with_pool")
	defer mh.UpdateSince(label, time.Now())

	h.instance = instance

	h.Printf("attaching user's volume %s to instance %s", h.Config.VolumeID, instance.InstanceId)
	if _, err := attachVolume(h.provider, h.Config.VolumeID, instance.InstanceId, userDeviceName); err != nil {
		return nil, fmt.Errorf("amazon: failed to attach user volume %s to %s, err: %v", h.Config.VolumeID, h.instance.InstanceId, err)
	}
	h.Printf("attached user's volume %s to instance %s", h.Config.VolumeID, instance.InstanceId)

	// Login to the instance and start docker and freighterd.
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), startTimeout)
	defer cancel()

	hctx := host.NewContext(ctx, h, h, nil)
	defer func() {
		if hctx.Executor != nil {
			hctx.Executor.Close()
		}
	}()

	steps := []host.Step{
		{
			Description: "Assigning network address",
			Func:        allocateElasticIP,
		},
		{
			Description: "Connecting to workspace",
			Func:        host.WaitForSSH,
			Timeout:     3 * time.Minute,
		},
		{
			Description: "Mounting user volume",
			Func:        mountUserVolume,
		},
		{
			Description: "Setting workspace's name",
			Func:        associateInstanceTags,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			h.Printf("ERROR: failed in step %q: %v", step.Description, err)

			if err == context.DeadlineExceeded {
				return nil, ErrTimeout
			}

			return nil, mh.Error(err, label)
		}
	}

	return h.Info()
}

// attachToNewInstance creates a new EC2 instance and attaches the given volume
// to the instance as its user volume.
func (h *Host) attachToNewInstance(volumeID string) (*host.Info, error) {
	label := h.MetricsLabel("start_with_pool.new_instance")
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

	steps := []host.Step{
		{
			Description: "Checking image's availability",
			Func:        inspectSourceAMI,
		},
		{
			Description: "Creating workspace",
			Func:        createInstance,
			Timeout:     5 * time.Minute,
		},
		{
			Description: "Setting workspace's name",
			Func:        associateInstanceTags,
		},
		{
			Func:    host.WaitForSSH,
			Timeout: 5 * time.Minute,
		},
		{
			Description: "Assigning network address",
			Func:        allocateElasticIP,
		},
		{
			Description: "Connecting via SSH",
			Func:        host.WaitForSSH,
			Timeout:     5 * time.Minute,
		},
		{
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
		{
			Description: "Unmount new volume",
			Func:        unmountUserVolume,
		},
		{
			Description: "Detaching new volume",
			Func:        detachUserVolume,
		},
		{
			Description: "Deleting new volume",
			Func:        deleteOldVolume,
		},
		{
			Description: "Attaching user's volume",
			Func: func(ctx *host.HostContext) error {
				h := ctx.Host.(*Host)

				instanceID := h.instance.InstanceId

				h.Printf("attaching user's volume %s to instance %s", volumeID, instanceID)
				vol, err := attachVolume(h.provider, volumeID, instanceID, userDeviceName)
				if err != nil {
					return fmt.Errorf("amazon: failed to attach user volume %s to %s, err: %v", volumeID, instanceID, err)
				}
				h.Printf("attached user's volume %s to instance %s", volumeID, instanceID)

				h.userVolume = vol

				return nil
			},
		},
		{
			Description: "Mounting user volume",
			Func:        mountUserVolume,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			h.Printf("ERROR: failed in step %q: %v", step.Description, err)

			if err == context.DeadlineExceeded {
				return nil, ErrTimeout
			}
			return nil, err
		}
	}

	return h.Info()
}
