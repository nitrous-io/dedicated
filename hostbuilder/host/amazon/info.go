package amazon

import (
	"errors"

	"github.com/nitrous-io/dedicated/hostbuilder/host"
)

func (h *Host) Info() (*host.Info, error) {
	if h.instance == nil {
		return nil, errors.New("Invalid ec2 instance")
	}

	if err := h.reloadInstance(h.instance.InstanceId); err != nil {
		return nil, err
	}

	info := &host.Info{
		Provider:       h.DriverName(),
		ID:             h.instance.InstanceId,
		IPAddress:      h.instance.PublicIpAddress,
		FreighterdPort: host.DefaultPort,
		Region:         h.provider.Region.Name,
		State:          getState(h.instance),
	}

	userVolume, err := h.getVolumeByDeviceName(userDeviceName)
	if err == nil {
		info.VolumeID = userVolume.VolumeId
	}

	// Return root volume ID if we don't have a user volume.
	// This is to support old hosts that only have a root volume.
	if info.VolumeID == "" {
		rootVolume, err := h.getVolumeByDeviceName(rootDeviceName)
		if err == nil {
			info.VolumeID = rootVolume.VolumeId
		}
	}

	return info, nil
}
