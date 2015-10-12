package amazon

import (
	"fmt"
	"time"

	"github.com/mitchellh/goamz/ec2"
)

type volumeProvider interface {
	Volumes(ids []string, filter *ec2.Filter) (*ec2.VolumesResp, error)
	DetachVolume(id string) (*ec2.SimpleResp, error)
	CreateVolume(options *ec2.CreateVolume) (*ec2.CreateVolumeResp, error)
	AttachVolume(volumeID, instanceID, deviceName string) (*ec2.AttachVolumeResp, error)

	Snapshots(ids []string, filter *ec2.Filter) (*ec2.SnapshotsResp, error)
	CreateSnapshot(id, description string) (*ec2.CreateSnapshotResp, error)
}

func detachVolume(provider volumeProvider, volumeID, instanceID string) (*ec2.Volume, error) {
	if _, err := provider.DetachVolume(volumeID); err != nil {
		return nil, fmt.Errorf("amazon: failed to detach volume %s, err: %v", volumeID, err)
	}

	// Wait for the volume to be detached.
	sc := &StateChangeConf{
		Pending: []string{"detaching", "attached"},
		Refresh: volumeAttachmentStateRefreshFunc(provider, volumeID, instanceID),
		Target:  "detached",
	}
	interval, tries := 2*time.Second, 60
	vol, err := WaitForStateWithInterval(sc, interval, tries)
	if err != nil {
		return nil, fmt.Errorf("amazon: failed waiting for volume %s to be detached, err: %v", volumeID, err)
	}

	return vol.(*ec2.Volume), nil
}

func createVolume(provider volumeProvider, snapshotID, availabilityZone string, size int64) (*ec2.Volume, error) {
	resp, err := provider.CreateVolume(&ec2.CreateVolume{
		AvailZone:  availabilityZone,
		Size:       size,
		SnapshotId: snapshotID,
		VolumeType: GeneralVolumeType,
	})

	// Wait for volume to be created.
	sc := &StateChangeConf{
		Pending: []string{"creating"},
		Refresh: volumeStateRefreshFunc(provider, resp.VolumeId),
		Target:  "available",
	}
	vol, err := WaitForState(sc)
	if err != nil {
		return nil, fmt.Errorf("amazon: failed waiting for volume to be created from snapshot %s, err: %v", snapshotID, err)
	}

	return vol.(*ec2.Volume), nil
}

func attachVolume(provider volumeProvider, volumeID, instanceID, deviceName string) (*ec2.Volume, error) {
	if _, err := provider.AttachVolume(volumeID, instanceID, deviceName); err != nil {
		return nil, fmt.Errorf("amazon: failed to attach volume %s to %s, err: %v", volumeID, instanceID, err)
	}

	// Wait for the volume to be attached.
	sc := &StateChangeConf{
		Pending: []string{"attaching", "detached"},
		Refresh: volumeAttachmentStateRefreshFunc(provider, volumeID, instanceID),
		Target:  "attached",
	}
	vol, err := WaitForState(sc)
	if err != nil {
		return nil, fmt.Errorf("amazon: failed waiting for volume %s to be attached, err: %v", volumeID, err)
	}

	return vol.(*ec2.Volume), nil
}

func createSnapshot(provider volumeProvider, volumeID, description string) (*ec2.Snapshot, error) {
	resp, err := provider.CreateSnapshot(volumeID, description)
	if err != nil {
		return nil, fmt.Errorf("amazon: failed to create snapshot of volume %s, err: %v", volumeID, err)
	}

	// Wait for snapshot to be completed.
	sc := &StateChangeConf{
		Pending: []string{"pending"},
		Refresh: snapshotStateRefreshFunc(provider, resp.Snapshot.Id),
		Target:  "completed",
	}
	snap, err := WaitForState(sc)
	if err != nil {
		return nil, fmt.Errorf("amazon: failed waiting for snapshot %+v to complete, err: %v", resp.Snapshot, err)
	}

	return snap.(*ec2.Snapshot), nil
}
