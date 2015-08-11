package amazon

import (
	"time"

	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
)

// Destroy will either:
// 1. for hosts prior to pre-EBS hotswapping, we release the EIP and terminate
//    the instance, or
// 2. for hosts that support EBS hotswapping, we delete the user volume. If the
//    volume is attached to an instance, we terminate the instance as well.
func (h *Host) Destroy() error {
	label := h.MetricsLabel("destroy")
	defer mh.UpdateSince(label, time.Now())

	h.Printf("destroy - checking if instance %s supports EBS hotswapping", h.Config.ID)

	// [Feature flag] - requires EBS hotswapping.
	vol, err := h.getVolume(h.Config.VolumeID)
	if err != nil {
		return mh.Error(err, label)
	}

	if h.supportsEBSHotswapping(vol) {
		return h.destroyWithPool()
	}

	h.Printf("destroying pre-EBS hotswapping instance normally")

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return mh.Error(err, label)
	}

	if err := h.terminateInstance(); err != nil {
		return mh.Error(err, label)
	}

	return nil
}

// destroyWithPool deletes the user volume. If the volume is attached to an
// instance, we terminate the instance and release any associated EIP.
func (h *Host) destroyWithPool() error {
	label := h.MetricsLabel("destroy_with_pool")
	defer mh.UpdateSince(label, time.Now())

	volumeID := h.Config.VolumeID

	// Check whether user volume is still attached to an instance.
	if h.instance != nil {
		userDevice := blockDevice(h.instance, userDeviceName)
		if userDevice != nil && userDevice.VolumeId == volumeID {
			h.Printf("user volume %s is still attached to instance %s, terminating that instance",
				volumeID, h.instance.InstanceId)
			if err := h.terminateInstance(); err != nil {
				return err
			}
		}
	}

	h.Printf("deleting user volume %s", volumeID)
	if _, err := h.provider.DeleteVolume(volumeID); err != nil {
		h.Printf("ERROR: failed to delete user volume %s, err: %v", volumeID, err)
		return err
	}
	h.Printf("deleted user volume %s", volumeID)

	return nil
}
