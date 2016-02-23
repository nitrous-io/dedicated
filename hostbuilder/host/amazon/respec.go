package amazon

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"golang.org/x/net/context"
)

// RespecWithQueue changes the specs of an existing host.
// The following changes are supported:
//   1. Changing storage size of user volume.
//   2. Changing instance type (only between allowable instance types, e.g. we
//      don't support changing from a VPC-only to non-VPC instance).
//   3. Allocating an Elastic IP.
func (h *Host) RespecWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	label := h.MetricsLabel("respec")
	defer mh.UpdateSince(label, time.Now())

	h.Printf("respec received: %+v", h.Config)

	host.PublishMessage(qp, event.HostRespeccing, h.requestID, queueInfo, "Respeccing host", queue.Respec)

	// If we don't have an ID (i.e. provider_identifier from nitrousd), the
	// host is a hotswap host that's stopped.
	hostStopped := (h.Config.ID == "")

	if !hostStopped {
		if err := h.reloadInstance(h.Config.ID); err != nil {
			return nil, mh.Error(err, label)
		}

		// Allocate Elastic IP.
		addresses, err := getAddresses(h.provider, h.instance.InstanceId)
		if err != nil {
			return nil, mh.Error(err, label)
		}
		// If instance already has an Elastic IP allocated, we skip this step.
		if len(addresses) == 0 {
			ctx, cancel := context.WithTimeout(context.Background(), allocateElasticIPTimeout)
			defer cancel()

			hctx := host.NewContext(ctx, h, h, nil)
			err := hctx.Run(host.Step{
				Description: "Allocating static IP",
				Func:        allocateElasticIP,
			})
			if err != nil {
				h.publishHostError(qp, queueInfo, event.HostRespecFailed, queue.Respec, err)
				h.Printf("ERROR: failed to allocate Elastic IP to instance %s: %v", h.instance.InstanceId, err)
				return nil, mh.Error(err, label)
			}
		}
	}

	h.Printf("respeccing hotswap instance")

	// If we don't have an ID (i.e. provider_identifier from nitrousd), the
	// host is a hotswap host that's stopped.
	if h.Config.ID == "" {
		info, err := h.respecStoppedHostWithPool(qp, queueInfo)

		// Tag volume (this is mostly to update its tags, if necessary).
		if err := h.tagVolume(info.VolumeID, "userdata"); err != nil {
			h.Printf("error tagging volume after respec, err: %v", err)
		}

		return info, err
	}

	h.Printf("respeccing running EBS hotswapping instance: %+v", h.instance)

	var changeInstanceType, changeStorageSize bool

	if h.Config.InstanceType != "" && h.Config.InstanceType != h.instance.InstanceType {
		changeInstanceType = true
	}

	var (
		userVolume     *ec2.Volume
		configuredSize int64
	)

	// If StorageSize is 0, no change required.
	if h.Config.StorageSize != 0 {
		var err error
		userVolume, err = h.getVolumeByDeviceName(userDeviceName)
		if err != nil {
			return nil, mh.Error(err, label)
		}

		currentSize, err := strconv.ParseInt(userVolume.Size, 10, 64)
		if err != nil {
			h.publishHostError(qp, queueInfo, event.HostRespecFailed, queue.Respec, err)
			return nil, mh.Error(fmt.Errorf("amazon: cannot parse size from volume: %+v, err: %v", userVolume, err), label)
		}

		configuredSize = computeSize(h.Config.StorageSize)
		if currentSize < configuredSize {
			changeStorageSize = true
			h.Printf("respec: volume resize required (%d to %d)", currentSize, configuredSize)
		}
	}

	// If no further changes required, we're done.
	if !changeInstanceType && !changeStorageSize {
		info, err := h.Info()
		if err == nil {
			if err := h.updateInstanceTags(); err != nil {
				h.Printf("error tagging instance after respec, err: %v", err)
			}
			host.PublishHost(qp, event.HostRespecced, h.requestID, queueInfo, info, queue.Respec, "")
		} else {
			logrus.WithField("uuid", h.requestID).Errorf("amazon: failed to send a message due to failure on getting host info: %v", err)
		}

		return info, err
	}

	if changeStorageSize {
		if err := h.liveResizeUserVolume(userVolume, configuredSize); err != nil {
			h.publishHostError(qp, queueInfo, event.HostRespecFailed, queue.Respec, err)
			return nil, mh.Error(fmt.Errorf("amazon: failed to resize user volume, err: %v", err), label)
		}
	}

	if changeInstanceType {
		// TODO We should try to acquire a new instance from the instance pool
		// of the new instance type.
		if err := h.stopInstance(); err != nil {
			h.publishHostError(qp, queueInfo, event.HostRespecFailed, queue.Respec, err)
			return nil, mh.Error(fmt.Errorf("amazon: failed to stop instance for respeccing, err: %v", err), label)
		}

		h.Printf("Changing instance type of instance %s from %s to %s", h.instance.InstanceId, h.instance.InstanceType, h.Config.InstanceType)
		if err := modifyInstanceType(h.provider, h, h.Config.InstanceType); err != nil {
			h.publishHostError(qp, queueInfo, event.HostRespecFailed, queue.Respec, err)
			return nil, mh.Error(fmt.Errorf("amazon: failed to change instance type, err: %v"), label)
		}

		if err := h.startInstance(); err != nil {
			h.publishHostError(qp, queueInfo, event.HostRespecFailed, queue.Respec, err)
			return nil, mh.Error(fmt.Errorf("amazon: failed to start instance for respeccing, err: %v", err), label)
		}
	}

	info, err := h.Info()
	if err == nil {
		host.PublishHost(qp, event.HostRespecced, h.requestID, queueInfo, info, queue.Respec, "")
	} else {
		logrus.WithField("uuid", h.requestID).Errorf("amazon: failed to send a message due to failure on getting host info: %v", err)
	}

	if err := h.updateInstanceTags(); err != nil {
		h.Printf("error tagging instance after respec, err: %v", err)
	}

	return info, err
}

func (h *Host) respecStoppedHostWithPool(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	label := h.MetricsLabel("respec_stopped_host")
	defer mh.UpdateSince(label, time.Now())

	h.Printf("respeccing stopped host, volume %s", h.Config.VolumeID)

	if h.Config.StorageSize == 0 {
		info := &host.Info{
			VolumeID: h.Config.VolumeID,
			Provider: h.DriverName(),
			Region:   h.provider.Region.Name,
			State:    state.Stopped,
		}
		host.PublishHost(qp, event.HostRespecced, h.requestID, queueInfo, info, queue.Respec, "")
		return info, nil
	}

	userVolume, err := h.getVolume(h.Config.VolumeID)
	if err != nil {
		return nil, mh.Error(err, label)
	}

	currentSize, err := strconv.ParseInt(userVolume.Size, 10, 64)
	if err != nil {
		h.publishHostError(qp, queueInfo, event.HostRespecFailed, queue.Respec, err)
		return nil, mh.Error(fmt.Errorf("amazon: cannot parse size from volume: %+v, err: %v", userVolume, err), label)
	}

	configuredSize := computeSize(h.Config.StorageSize)
	if currentSize >= configuredSize {
		h.Printf("respec: volume resize not required, current size is already %d", currentSize)

		info := &host.Info{
			VolumeID: h.Config.VolumeID,
			Provider: h.DriverName(),
			Region:   h.provider.Region.Name,
			State:    state.Stopped,
		}
		host.PublishHost(qp, event.HostRespecced, h.requestID, queueInfo, info, queue.Respec, "")
		return info, nil
	}

	h.Printf("respec: volume resize required (%d to %d)", currentSize, configuredSize)

	if err := h.resizeUserVolume(userVolume, configuredSize); err != nil {
		h.publishHostError(qp, queueInfo, event.HostRespecFailed, queue.Respec, err)
		return nil, mh.Error(fmt.Errorf("amazon: failed to resize user volume, err: %v", err), label)
	}

	info := &host.Info{
		VolumeID: h.newVolume.VolumeId, // Use new volume's ID.
		Provider: h.DriverName(),
		Region:   h.provider.Region.Name,
		State:    state.Stopped,
	}

	host.PublishHost(qp, event.HostRespecced, h.requestID, queueInfo, info, queue.Respec, "")

	return info, nil
}

// updateInstanceTags tags the host's EC2 instance with configured tags, if any.
func (h *Host) updateInstanceTags() error {
	if h.Config.Tags == "" {
		return nil
	}

	tags, err := parseTags(h.Config.Tags)
	if err != nil {
		h.Printf("error parsing tags %q, err: %v", h.Config.Tags, err)
		return err
	}

	instanceID := h.instance.InstanceId
	h.Printf("tagging instance %s with tags %#v", instanceID, tags)
	if _, err := h.provider.CreateTags([]string{instanceID}, tags); err != nil {
		return fmt.Errorf("amazon: failed to tag instance %s with tags %#v, err: %v", instanceID, tags, err)
	}

	return nil
}

type resizeStep struct {
	host.Step
	restartOnError bool
}

func (h *Host) resizeRootVolume(rootVolume *ec2.Volume, newSize int64) (err error) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), resizeVolumeTimeout)
	defer cancel()

	h.rootVolume = rootVolume
	h.newSize = newSize

	hctx := host.NewContext(ctx, h, h, nil)
	steps := []resizeStep{
		{
			Step: host.Step{
				Description: "Taking snapshot of disk",
				Func:        snapshotRootVolume,
			},
			restartOnError: true,
		},
		{
			Step: host.Step{
				Description: "Creating larger disk",
				Func:        createNewRootVolume,
			},
			restartOnError: true,
		},
		{
			Step: host.Step{
				Description: "Switching to larger disk",
				Func:        swapRootVolume,
			},
		},
		{
			Step: host.Step{
				Description: "Cleaning up",
				Func:        deleteOldRootVolume,
			},
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step.Step); err != nil {
			h.Printf("ERROR: failed in resize root volume step %q: %v", step.Description, err)

			// Start the instance back up if necessary.
			// Note that the time to start the instance is excluded from the
			// context timeout.
			if step.restartOnError {
				if e := h.startInstance(); e != nil {
					h.Printf("failed to start instance %s back up", h.instance.InstanceId)
				}
			}

			if err == context.DeadlineExceeded {
				return ErrCreateTimeout
			}
			return err
		}
	}

	return nil
}

// resizeUserVolume resizes user volume that's not attached to an instance.
func (h *Host) resizeUserVolume(userVolume *ec2.Volume, newSize int64) (err error) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), resizeVolumeTimeout)
	defer cancel()

	h.userVolume = userVolume
	h.newSize = newSize

	hctx := host.NewContext(ctx, h, h, nil)
	steps := []host.Step{
		{
			Description: "Taking snapshot of current user volume",
			Func:        snapshotUserVolume,
		},
		{
			Description: "Creating larger volume",
			Func:        createNewVolume,
		},
		{
			Description: "Cleaning up",
			Func:        deleteOldVolume,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			h.Printf("ERROR: failed in resize user volume step %q: %v", step.Description, err)

			if err == context.DeadlineExceeded {
				return ErrTimeout
			}
			return err
		}
	}

	return nil
}

// liveResizeUserVolume resizes user volume of a running instance.
func (h *Host) liveResizeUserVolume(userVolume *ec2.Volume, newSize int64) (err error) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), resizeVolumeTimeout)
	defer cancel()

	h.userVolume = userVolume
	h.newSize = newSize

	hctx := host.NewContext(ctx, h, h, nil)
	steps := []host.Step{
		{
			Description: "Connecting to workspace",
			Func:        host.WaitForSSH,
			Timeout:     3 * time.Minute,
		},
		{
			Description: "Unmounting current user volume",
			Func:        unmountUserVolume,
		},
		{
			Description: "Taking snapshot of current user volume",
			Func:        snapshotUserVolume,
		},
		{
			Description: "Creating larger volume",
			Func:        createNewVolume,
		},
		{
			Description: "Switching to new user volume",
			Func:        swapUserVolume,
		},
		{
			Description: "Mounting new user volume",
			Func:        mountUserVolume,
		},
		{
			Description: "Cleaning up",
			Func:        deleteOldVolume,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			h.Printf("ERROR: failed in resize user volume step %q: %v", step.Description, err)

			if err == context.DeadlineExceeded {
				return ErrTimeout
			}
			return err
		}
	}

	return nil
}

func snapshotRootVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("snapshot_root_volume")
	defer mh.UpdateSince(label, time.Now())

	rootVolume := h.rootVolume
	newSize := h.newSize

	h.Printf("Creating snapshot of root volume %s of instance %s", rootVolume.VolumeId, h.instance.InstanceId)

	desc := fmt.Sprintf("Created by hostbuilder for respec of %s from %sGB to %dGB",
		h.instance.InstanceId, rootVolume.Size, newSize)
	snapshot, err := createSnapshot(h.provider, rootVolume.VolumeId, desc)
	if err != nil {
		return fmt.Errorf("amazon: failed to create snapshot of root volume %+v, err: %v", rootVolume, err)
	}

	h.snapshot = snapshot

	tags := []ec2.Tag{
		{Key: "InstanceID", Value: h.instance.InstanceId},
		{Key: "dedicated", Value: h.driverConfig.Env},
	}
	if _, err := h.provider.CreateTags([]string{snapshot.Id}, tags); err != nil {
		h.Printf("ERROR: failed to tag snapshot %v, err: %v", snapshot.Id, err)
	}

	h.Printf("Created snapshot %s of volume %s", snapshot.Id, rootVolume.VolumeId)

	return nil
}

func createNewRootVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("create_new_volume")
	defer mh.UpdateSince(label, time.Now())

	rootVolume := h.rootVolume
	snapshot := h.snapshot
	newSize := h.newSize

	h.Printf("Creating volume from snapshot %s", snapshot.Id)
	newVolume, err := createVolume(h.provider, snapshot.Id, rootVolume.AvailZone, newSize)
	if err != nil {
		return fmt.Errorf("amazon: failed to create new volume from snapshot %s, err: %v", snapshot.Id, err)
	}
	h.Printf("Created new volume %s from snapshot %s", newVolume.VolumeId, snapshot.Id)

	h.newVolume = newVolume

	return h.tagVolume(newVolume.VolumeId, "userroot")
}

func swapRootVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("swap_root_volume")
	defer mh.UpdateSince(label, time.Now())

	rootVolume := h.rootVolume
	newVolume := h.newVolume

	h.Printf("Detaching root volume %s of instance %s", rootVolume.VolumeId, h.instance.InstanceId)
	if vol, err := detachVolume(h.provider, rootVolume.VolumeId, h.instance.InstanceId); err != nil {
		return fmt.Errorf("amazon: failed to detach root volume, err: %v", err)
	} else {
		rootVolume = vol
	}
	h.Printf("Detached root volume %s from %s", rootVolume.VolumeId, h.instance.InstanceId)

	h.Printf("Attaching new volume %s to instance %s", newVolume.VolumeId, h.instance.InstanceId)
	if vol, err := attachVolume(h.provider, newVolume.VolumeId, h.instance.InstanceId, rootDeviceName); err != nil {
		return fmt.Errorf("amazon: failed to attach new volume %+v to %s, err: %v", newVolume, h.instance.InstanceId, err)
	} else {
		newVolume = vol
	}
	h.Printf("Attached new volume %s to instance %s", newVolume.VolumeId, h.instance.InstanceId)

	return nil
}

func deleteOldRootVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("delete_old_volume")
	defer mh.UpdateSince(label, time.Now())

	rootVolume := h.rootVolume

	h.Printf("Deleting old root volume %s of instance %s", rootVolume.VolumeId, h.instance.InstanceId)
	if _, err := h.provider.DeleteVolume(rootVolume.VolumeId); err != nil {
		h.Printf("ERROR: failed to delete old root volume %s for instance %s after respec, err: %v", rootVolume.VolumeId, h.instance.InstanceId, err)
	}
	h.Printf("Deleted old root volume %s of instance %s", rootVolume.VolumeId, h.instance.InstanceId)

	return nil
}

func snapshotUserVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("snapshot_user_volume")
	defer mh.UpdateSince(label, time.Now())

	userVolume := h.userVolume
	newSize := h.newSize

	h.Printf("Creating snapshot of user volume %s", userVolume.VolumeId)

	desc := fmt.Sprintf("Created by hostbuilder for respec of %s from %sGB to %dGB",
		userVolume.VolumeId, userVolume.Size, newSize)
	snapshot, err := createSnapshot(h.provider, userVolume.VolumeId, desc)
	if err != nil {
		return fmt.Errorf("amazon: failed to create snapshot of user volume %+v, err: %v", userVolume, err)
	}

	h.snapshot = snapshot

	tags := []ec2.Tag{
		{Key: "volume-id", Value: userVolume.VolumeId},
		{Key: "dedicated", Value: h.driverConfig.Env},
	}
	if _, err := h.provider.CreateTags([]string{snapshot.Id}, tags); err != nil {
		h.Printf("ERROR: failed to tag snapshot %v, err: %v", snapshot.Id, err)
	}

	h.Printf("Created snapshot %s of volume %s", snapshot.Id, userVolume.VolumeId)

	return nil
}

func createNewVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("create_new_volume")
	defer mh.UpdateSince(label, time.Now())

	userVolume := h.userVolume
	snapshot := h.snapshot
	newSize := h.newSize

	h.Printf("Creating volume from snapshot %s", snapshot.Id)
	newVolume, err := createVolume(h.provider, snapshot.Id, userVolume.AvailZone, newSize)
	if err != nil {
		return fmt.Errorf("amazon: failed to create new volume from snapshot %s, err: %v", snapshot.Id, err)
	}
	h.Printf("Created new volume %s from snapshot %s", newVolume.VolumeId, snapshot.Id)

	if err := h.tagVolume(newVolume.VolumeId, "userdata"); err != nil {
		return err
	}

	h.newVolume = newVolume

	return nil
}

func swapUserVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("swap_user_volume")
	defer mh.UpdateSince(label, time.Now())

	userVolume := h.userVolume
	newVolume := h.newVolume

	h.Printf("Detaching user volume %s of instance %s", userVolume.VolumeId, h.instance.InstanceId)
	if vol, err := detachVolume(h.provider, userVolume.VolumeId, h.instance.InstanceId); err != nil {
		return fmt.Errorf("amazon: failed to detach user volume, err: %v", err)
	} else {
		userVolume = vol
	}
	h.Printf("Detached user volume %s from %s", userVolume.VolumeId, h.instance.InstanceId)

	h.Printf("Attaching new volume %s to instance %s", newVolume.VolumeId, h.instance.InstanceId)
	if vol, err := attachVolume(h.provider, newVolume.VolumeId, h.instance.InstanceId, userDeviceName); err != nil {
		return fmt.Errorf("amazon: failed to attach new volume %+v to %s, err: %v", newVolume, h.instance.InstanceId, err)
	} else {
		newVolume = vol
	}
	h.Printf("Attached new volume %s to instance %s", newVolume.VolumeId, h.instance.InstanceId)

	return nil
}

func deleteOldVolume(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("delete_old_volume")
	defer mh.UpdateSince(label, time.Now())

	userVolume := h.userVolume

	h.Printf("Deleting old user volume %s", userVolume.VolumeId)
	if _, err := h.provider.DeleteVolume(userVolume.VolumeId); err != nil {
		h.Printf("ERROR: failed to delete old user volume %s after respec, err: %v", userVolume.VolumeId, err)
	}
	h.Printf("Deleted old user volume %s", userVolume.VolumeId)

	return nil
}
