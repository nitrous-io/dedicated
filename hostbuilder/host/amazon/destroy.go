package amazon

import (
	"time"

	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
)

// Destroy will delete the user volume. If the volume is attached to an
// instance, we terminate the instance as well.
func (h *Host) Destroy() error {
	label := h.MetricsLabel("destroy")
	defer mh.UpdateSince(label, time.Now())

	h.Printf("destroying hotswap instance")

	volumeID := h.Config.VolumeID

	// Check whether user volume is still attached to an instance.
	if h.Config.ID != "" {
		if err := h.reloadInstance(h.Config.ID); err != nil {
			return mh.Error(err, label)
		}

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
