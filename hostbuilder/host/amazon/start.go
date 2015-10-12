package amazon

import (
	"errors"
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
	h.Printf("checking if volume is detached")
	vol, err := h.getVolume(h.Config.VolumeID)
	if err != nil {
		errstr := fmt.Sprintf("could not retrieve volume info for %s, err: %v",
			h.Config.VolumeID, err)
		h.Printf(errstr)
		return nil, errors.New(errstr)
	}
	if len(vol.Attachments) != 0 {
		return nil, fmt.Errorf("amazon: failed to start as volume %s is currently attached to %s",
			vol.VolumeId, vol.Attachments[0].InstanceId)
	}

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
			Func:        attachElasticIP,
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

			h.Printf("attempting to recover from start failure")
			if err := h.recoverFromStart(); err != nil {
				h.Printf("failed to recover from start failure, err: %v", err)
			}

			if err == context.DeadlineExceeded {
				return nil, ErrTimeout
			}

			return nil, mh.Error(err, label)
		}
	}

	return h.Info()
}

type stepWithRecovery struct {
	host.Step
	RecoveryFunc func(*host.HostContext) error
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

	steps := []stepWithRecovery{
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
		{
			Step: host.Step{
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
		},
		{
			Step: host.Step{
				Description: "Mounting user volume",
				Func:        mountUserVolume,
			},
			RecoveryFunc: func(ctx *host.HostContext) error {
				h := ctx.Host.(*Host)
				return h.recoverFromStart()
			},
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step.Step); err != nil {
			h.Printf("ERROR: failed in step %q: %v", step.Description, err)

			if step.RecoveryFunc != nil {
				h.Printf("attempting to recover from start failure in step %q", step.Step.Description)
				if err := step.RecoveryFunc(hctx); err != nil {
					h.Printf("failed to recover from start failure, err: %v", err)
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

// recoverFromStart attempts to recover from an error from any of the steps that
// make up a Start operation by:
// 1. Detaching elastic IP if it was attached.
// 2. Stopping freighterd if it was started.
// 3. Stopping docker if it was started.
// 4. Unmounting user device if it was mounted.
// 5. Detaching user volume if it was attached.
func (h *Host) recoverFromStart() error {
	instanceID := h.instance.InstanceId
	volumeID := h.Config.VolumeID

	vol, err := h.getVolume(volumeID)
	if err != nil {
		h.Printf("Start() recovery: failed to get volume %s info, err: %v", volumeID, err)
		return err
	}

	if len(vol.Attachments) == 0 {
		// If volume is not attached, we are done.
		h.Printf("Start() recovery: volume %s is not attached", volumeID)
		return nil
	}

	hctx := host.NewContext(context.Background(), h, h, nil)

	if err := host.WaitForSSH(hctx); err != nil {
		h.Printf("Start() recovery: failed to SSH into instance %s, err: %v", instanceID, err)
		return err
	}
	defer hctx.Executor.Close()

	h.Printf("Start() recovery: stopping freighterd if necessary")
	fCmd := fmt.Sprintf(`sudo service freighterd status | grep -q 'freighterd stop' || sudo service freighterd stop`)
	if out, err := hctx.Executor.CombinedOutput(fCmd); err != nil {
		h.Printf("Start() recovery: failed to stop freighterd on instance %s, err: %v, output: %s",
			instanceID, err, out)
		return fmt.Errorf("failed to stop freighterd, err: %v, output: %s", err, out)
	}

	h.Printf("Start() recovery: stopping docker if necessary")
	dCmd := fmt.Sprintf(`sudo service docker status | grep -q 'docker stop' || sudo service docker stop`)
	if out, err := hctx.Executor.CombinedOutput(dCmd); err != nil {
		h.Printf("Start() recovery: failed to stop docker on instance %s, err: %v, output: %s",
			instanceID, err, out)
		return fmt.Errorf("failed to stop docker, err: %v, output: %s", err, out)
	}

	h.Printf("Stop() recovery: unmounting %s if necessary", userDeviceOSName)
	unmountCmd := fmt.Sprintf(`if sudo grep -qs '^%s\s' /proc/mounts; then sudo umount %s; else true; fi`, userDeviceOSName, userDeviceOSName)
	if out, err := hctx.Executor.CombinedOutput(unmountCmd); err != nil {
		h.Printf("Stop() recovery: failed to unmount %s on instance %s, err: %v, output: %s",
			userDeviceOSName, instanceID, err, out)
		return err
	}

	h.Printf("Start() recovery: detaching user's volume %s from instance %s", volumeID, instanceID)
	if _, err := detachVolume(h.provider, volumeID, instanceID); err != nil {
		return fmt.Errorf("amazon: failed to detach user volume %s from %s, err: %v", volumeID, instanceID, err)
	}
	h.Printf("Start() recovery: detached user's volume %s from instance %s", volumeID, instanceID)

	if err := h.recoverAttachElasticIP(); err != nil {
		return err
	}

	return nil
}

func (h *Host) recoverAttachElasticIP() error {
	if h.Config.DisableEIP {
		return nil
	}

	instanceID := h.instance.InstanceId
	ip := h.Config.IPAddress

	// Check whether IP is attached to the instance, and detach it if so.
	addresses, err := getAddresses(h.provider, instanceID)
	if err != nil {
		return fmt.Errorf("Start() recovery: failed to get IP addresses for instance %s, err: %v", instanceID, err)
	}
	for _, addr := range addresses {
		if ip == addr.PublicIp {
			if err := disassociateAddress(h.provider, ip, isVPC(h.instance)); err != nil {
				return fmt.Errorf("Start() recovery: failed to detach IP address %s from instance %s, err: %v", ip, instanceID, err)
			}
			break
		}
	}

	return nil
}
