package amazon

import (
	"strconv"

	"github.com/mitchellh/goamz/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type dummyVolumeProvider struct {
	volumes   []ec2.Volume
	snapshots []ec2.Snapshot
	calls     []fn
}

func (p *dummyVolumeProvider) Volumes(ids []string, filter *ec2.Filter) (*ec2.VolumesResp, error) {
	p.calls = append(p.calls, fn{"Volumes", []interface{}{ids, filter}})
	vols := []ec2.Volume{}
	for _, v := range p.volumes {
		for _, id := range ids {
			if v.VolumeId == id {
				vols = append(vols, v)
			}
		}
	}
	return &ec2.VolumesResp{Volumes: vols}, nil
}

func (p *dummyVolumeProvider) DetachVolume(id string) (*ec2.SimpleResp, error) {
	p.calls = append(p.calls, fn{"DetachVolume", []interface{}{id}})
	return &ec2.SimpleResp{}, nil
}

func (p *dummyVolumeProvider) CreateVolume(options *ec2.CreateVolume) (*ec2.CreateVolumeResp, error) {
	p.calls = append(p.calls, fn{"CreateVolume", []interface{}{options}})

	p.volumes = append(p.volumes, ec2.Volume{
		VolumeId:   "vol-test",
		SnapshotId: options.SnapshotId,
		AvailZone:  options.AvailZone,
		Size:       strconv.FormatInt(options.Size, 10),
		VolumeType: options.VolumeType,
		Status:     "available",
	})

	return &ec2.CreateVolumeResp{
		VolumeId:   "vol-test",
		SnapshotId: options.SnapshotId,
		AvailZone:  options.AvailZone,
		Size:       options.Size,
		VolumeType: options.VolumeType,
		Status:     "creating",
	}, nil
}

func (p *dummyVolumeProvider) AttachVolume(volumeID, instanceID, deviceName string) (*ec2.AttachVolumeResp, error) {
	p.calls = append(p.calls, fn{"AttachVolume", []interface{}{volumeID, instanceID, deviceName}})

	p.volumes = append(p.volumes, ec2.Volume{
		VolumeId: volumeID,
		Attachments: []ec2.VolumeAttachment{
			{
				VolumeId:   volumeID,
				InstanceId: instanceID,
				Device:     deviceName,
				Status:     "attached",
			},
		},
	})

	return &ec2.AttachVolumeResp{
		VolumeId:   volumeID,
		InstanceId: instanceID,
		Device:     deviceName,
		Status:     "attaching",
	}, nil
}

func (p *dummyVolumeProvider) Snapshots(ids []string, filter *ec2.Filter) (*ec2.SnapshotsResp, error) {
	p.calls = append(p.calls, fn{"Snapshots", []interface{}{ids, filter}})
	return &ec2.SnapshotsResp{Snapshots: p.snapshots}, nil
}

func (p *dummyVolumeProvider) CreateSnapshot(id, description string) (*ec2.CreateSnapshotResp, error) {
	p.calls = append(p.calls, fn{"CreateSnapshot", []interface{}{id, description}})

	snapshot := ec2.Snapshot{
		Id:          "snap-test",
		VolumeId:    id,
		Status:      "completed",
		Description: description,
	}
	p.snapshots = append(p.snapshots, snapshot)

	return &ec2.CreateSnapshotResp{Snapshot: snapshot}, nil
}

var _ = Describe("detachVolume", func() {
	It("detaches a known volume", func() {
		p := &dummyVolumeProvider{
			volumes: []ec2.Volume{
				{VolumeId: "vol-abcd"},
			},
		}

		volumeID := "vol-abcd"
		instanceID := "i-1234"
		_, err := detachVolume(p, volumeID, instanceID)
		Expect(err).To(BeNil())

		Expect(p.calls[0].name).To(Equal("DetachVolume"))
		Expect(p.calls[0].args).To(Equal([]interface{}{"vol-abcd"}))
	})

	It("returns an error when trying to detach an unknown volume", func() {
		p := &dummyVolumeProvider{} // No volumes.

		volumeID := "vol-abcd"
		instanceID := "i-1234"
		_, err := detachVolume(p, volumeID, instanceID)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("createVolume", func() {
	It("creates a volume", func() {
		p := &dummyVolumeProvider{}

		availabilityZone := "us-west-1a"
		size := int64(50)
		vol, err := createVolume(p, "snap-zxcv", availabilityZone, size)
		Expect(err).To(BeNil())

		Expect(vol.SnapshotId).To(Equal("snap-zxcv"))
		Expect(vol.AvailZone).To(Equal(availabilityZone))
		Expect(vol.Size).To(Equal("50"))
		Expect(vol.VolumeType).To(Equal(GeneralVolumeType))
		Expect(vol.Status).To(Equal("available"))
	})
})

var _ = Describe("attachVolume", func() {
	It("attaches a volume", func() {
		p := &dummyVolumeProvider{}

		volumeID := "vol-abcd"
		instanceID := "i-1234"
		vol, err := attachVolume(p, volumeID, instanceID, "/dev/sda1")
		Expect(err).To(BeNil())

		Expect(vol.VolumeId).To(Equal(volumeID))
		Expect(vol.Attachments).To(HaveLen(1))

		attachment := vol.Attachments[0]
		Expect(attachment.InstanceId).To(Equal(instanceID))
		Expect(attachment.Device).To(Equal("/dev/sda1"))
		Expect(attachment.Status).To(Equal("attached"))
	})
})

var _ = Describe("createSnapshot", func() {
	It("creates a snapshot of given volume", func() {
		p := &dummyVolumeProvider{}

		volumeID := "vol-abcd"
		snapshot, err := createSnapshot(p, volumeID, "some description of snapshot")
		Expect(err).To(BeNil())

		Expect(snapshot.VolumeId).To(Equal(volumeID))
	})
})
