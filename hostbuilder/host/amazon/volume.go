package amazon

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type volumeProvider interface {
	DescribeVolumes(*ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error)
	DetachVolume(*ec2.DetachVolumeInput) (*ec2.VolumeAttachment, error)
	CreateVolume(*ec2.CreateVolumeInput) (*ec2.Volume, error)
	AttachVolume(*ec2.AttachVolumeInput) (*ec2.VolumeAttachment, error)

	DescribeSnapshots(*ec2.DescribeSnapshotsInput) (*ec2.DescribeSnapshotsOutput, error)
	CreateSnapshot(*ec2.CreateSnapshotInput) (*ec2.Snapshot, error)
}

// getVolume returns an ec2 volume given its ID. The volume returned without
// error is guaranteed to have valid size, volume id and availability zone.
func getVolume(provider volumeProvider, volumeID *string) (*ec2.Volume, error) {
	if volumeID == nil {
		return nil, fmt.Errorf("amazon: getVolume invalid arguments")
	}
	resp, err := provider.DescribeVolumes(&ec2.DescribeVolumesInput{
		VolumeIDs: []*string{volumeID},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Volumes) != 1 {
		return nil, fmt.Errorf("amazon: expected to get 1 volume but got %+v", resp.Volumes)
	}
	v := resp.Volumes[0]
	if v.VolumeID == nil || v.Size == nil || v.AvailabilityZone == nil || *v.VolumeID != *volumeID {
		// Sanity check - because using the wrong volume is bad m'kay.
		return nil, fmt.Errorf("amazon: expected volume %s but got volume: %+v", *volumeID, v)
	}
	return v, nil
}

func detachVolume(provider volumeProvider, volumeID, instanceID *string) (*ec2.Volume, error) {
	if volumeID == nil || instanceID == nil {
		return nil, fmt.Errorf("amazon: detachVolume invalid arguments")
	}
	if _, err := provider.DetachVolume(&ec2.DetachVolumeInput{
		VolumeID:   volumeID,
		InstanceID: instanceID,
	}); err != nil {
		return nil, fmt.Errorf("amazon: failed to detach volume %s, err: %v", *volumeID, err)
	}

	// Wait for the volume to be detached.
	sc := &StateChangeConf{
		Pending: []string{"detaching", "attached"},
		Refresh: volumeAttachmentStateRefreshFunc(provider, volumeID, instanceID),
		Target:  "detached",
	}
	resp, err := WaitForState(sc)
	if err != nil {
		return nil, fmt.Errorf("amazon: failed waiting for volume %s to be detached, err: %v", *volumeID, err)
	}
	v, ok := resp.(*ec2.Volume)
	if !ok || v.VolumeID == nil {
		return nil, fmt.Errorf("amazon: invalid volume %+v", v)
	}
	return v, nil
}

func createVolume(provider volumeProvider, snapshotID, availabilityZone *string, size int64) (*ec2.Volume, error) {
	if snapshotID == nil || availabilityZone == nil || size <= 0 {
		return nil, fmt.Errorf("amazon: createVolume invalid arguments")
	}
	resp, err := provider.CreateVolume(&ec2.CreateVolumeInput{
		AvailabilityZone: availabilityZone,
		Size:             aws.Long(size),
		SnapshotID:       snapshotID,
		VolumeType:       GeneralVolumeType,
	})
	if resp.VolumeID == nil {
		return nil, fmt.Errorf("amazon: failed to create volume from snapshot %s", *snapshotID)
	}
	// Wait for volume to be created.
	sc := &StateChangeConf{
		Pending: []string{"creating"},
		Refresh: volumeStateRefreshFunc(provider, resp.VolumeID),
		Target:  "available",
	}
	volResp, err := WaitForState(sc)
	if err != nil {
		return nil, fmt.Errorf("amazon: failed waiting for volume to be created from snapshot %s, err: %v", *snapshotID, err)
	}
	v, ok := volResp.(*ec2.Volume)
	if !ok || v.VolumeID == nil {
		return nil, fmt.Errorf("amazon: invalid volume %+v", v)
	}
	return v, nil
}

func attachVolume(provider volumeProvider, volumeID, instanceID, deviceName *string) (*ec2.Volume, error) {
	if volumeID == nil || instanceID == nil || deviceName == nil {
		return nil, fmt.Errorf("amazon: attachVolume invalid arguments")
	}
	if _, err := provider.AttachVolume(
		&ec2.AttachVolumeInput{
			Device:     deviceName,
			InstanceID: instanceID,
			VolumeID:   volumeID,
		}); err != nil {
		return nil, fmt.Errorf("amazon: failed to attach volume %s to %s, err: %v", *volumeID, *instanceID, err)
	}

	// Wait for the volume to be attached.
	sc := &StateChangeConf{
		Pending: []string{"attaching", "detached"},
		Refresh: volumeAttachmentStateRefreshFunc(provider, volumeID, instanceID),
		Target:  "attached",
	}
	resp, err := WaitForState(sc)
	if err != nil {
		return nil, fmt.Errorf("amazon: failed waiting for volume %s to be attached, err: %v", volumeID, err)
	}
	v, ok := resp.(*ec2.Volume)
	if !ok || v.VolumeID == nil {
		return nil, fmt.Errorf("amazon: invalid volume %+v", v)
	}
	return v, nil
}

// createSnapshot returns a snapshot of a given volume with a valid snapshot ID
// when it succesfully snapshotted the volume.
func createSnapshot(provider volumeProvider, volumeID, description *string) (*ec2.Snapshot, error) {
	if volumeID == nil || description == nil {
		return nil, fmt.Errorf("amazon: createSnapshot invalid arguments")
	}
	snapshot, err := provider.CreateSnapshot(&ec2.CreateSnapshotInput{
		VolumeID:    volumeID,
		Description: description,
	})
	if err != nil {
		return nil, fmt.Errorf("amazon: failed to create snapshot of volume %s, err: %v", volumeID, err)
	}
	if snapshot.SnapshotID == nil {
		return nil, fmt.Errorf("amazon: invalid new snapshot of volume %s", volumeID)
	}
	// Wait for snapshot to be completed.
	sc := &StateChangeConf{
		Pending: []string{"pending"},
		Refresh: snapshotStateRefreshFunc(provider, snapshot.SnapshotID),
		Target:  "completed",
	}
	resp, err := WaitForState(sc)
	if err != nil {
		return nil, fmt.Errorf("amazon: failed waiting for snapshot %+v to complete, err: %v", snapshot, err)
	}
	s, ok := resp.(*ec2.Snapshot)
	if !ok || s.SnapshotID == nil {
		return nil, fmt.Errorf("amazon: invalid snapshot %+v", s)
	}
	return s, nil
}
