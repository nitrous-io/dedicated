package amazon

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type dummyVolumeProvider struct {
	volumes   []*ec2.Volume
	snapshots []*ec2.Snapshot
	calls     []fn
}

func (p *dummyVolumeProvider) DescribeVolumes(req *ec2.DescribeVolumesInput) (*ec2.DescribeVolumesOutput, error) {
	p.calls = append(p.calls, fn{"Volumes", []interface{}{req}})
	vols := []*ec2.Volume{}
	for _, v := range p.volumes {
		for _, id := range req.VolumeIDs {
			if *v.VolumeID == *id {
				vols = append(vols, v)
			}
		}
	}
	return &ec2.DescribeVolumesOutput{Volumes: vols}, nil
}

func (p *dummyVolumeProvider) DetachVolume(req *ec2.DetachVolumeInput) (*ec2.VolumeAttachment, error) {
	p.calls = append(p.calls, fn{"DetachVolume", []interface{}{req}})
	return &ec2.VolumeAttachment{}, nil
}

func (p *dummyVolumeProvider) CreateVolume(req *ec2.CreateVolumeInput) (*ec2.Volume, error) {
	p.calls = append(p.calls, fn{"CreateVolume", []interface{}{req}})

	p.volumes = append(p.volumes, &ec2.Volume{
		VolumeID:         aws.String("vol-test"),
		SnapshotID:       req.SnapshotID,
		AvailabilityZone: req.AvailabilityZone,
		Size:             req.Size,
		VolumeType:       req.VolumeType,
		State:            aws.String("available"),
	})

	return &ec2.Volume{
		VolumeID:         aws.String("vol-test"),
		SnapshotID:       req.SnapshotID,
		AvailabilityZone: req.AvailabilityZone,
		Size:             req.Size,
		VolumeType:       req.VolumeType,
		State:            aws.String("creating"),
	}, nil
}

func (p *dummyVolumeProvider) AttachVolume(req *ec2.AttachVolumeInput) (*ec2.VolumeAttachment, error) {
	p.calls = append(p.calls, fn{"AttachVolume", []interface{}{req}})

	for _, v := range p.volumes {
		if *v.VolumeID == *req.VolumeID {
			v.Attachments = []*ec2.VolumeAttachment{
				{
					VolumeID:   req.VolumeID,
					InstanceID: req.InstanceID,
					Device:     req.Device,
					State:      aws.String("attached"),
				},
			}
		}
	}

	return &ec2.VolumeAttachment{
		VolumeID:   req.VolumeID,
		InstanceID: req.InstanceID,
		Device:     req.Device,
		State:      aws.String("attaching"),
	}, nil
}

func (p *dummyVolumeProvider) DescribeSnapshots(req *ec2.DescribeSnapshotsInput) (*ec2.DescribeSnapshotsOutput, error) {
	p.calls = append(p.calls, fn{"Snapshots", []interface{}{req}})
	return &ec2.DescribeSnapshotsOutput{Snapshots: p.snapshots}, nil
}

func (p *dummyVolumeProvider) CreateSnapshot(req *ec2.CreateSnapshotInput) (*ec2.Snapshot, error) {
	p.calls = append(p.calls, fn{"CreateSnapshot", []interface{}{req}})

	snapshot := &ec2.Snapshot{
		SnapshotID:  aws.String("snap-test"),
		VolumeID:    req.VolumeID,
		State:       aws.String("completed"),
		Description: req.Description,
	}
	p.snapshots = append(p.snapshots, snapshot)

	return snapshot, nil
}

var _ = Describe("detachVolume", func() {
	It("detaches a known volume", func() {
		p := &dummyVolumeProvider{
			volumes: []*ec2.Volume{
				{
					VolumeID: aws.String("vol-abcd"),
					Attachments: []*ec2.VolumeAttachment{
						{
							InstanceID: aws.String("i-1234"),
							State:      aws.String("detached"),
						},
					},
				},
			},
		}

		volumeID := aws.String("vol-abcd")
		instanceID := aws.String("i-1234")
		_, err := detachVolume(p, volumeID, instanceID)
		Expect(err).To(BeNil())

		Expect(p.calls[0].name).To(Equal("DetachVolume"))
		req := p.calls[0].args[0].(*ec2.DetachVolumeInput)
		Expect(*req.VolumeID).To(Equal("vol-abcd"))
	})

	It("returns an error when trying to detach an unknown volume", func() {
		p := &dummyVolumeProvider{} // No volumes.

		volumeID := aws.String("vol-abcd")
		instanceID := aws.String("i-1234")
		_, err := detachVolume(p, volumeID, instanceID)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("createVolume", func() {
	It("creates a volume", func() {
		p := &dummyVolumeProvider{}

		availabilityZone := aws.String("us-west-1a")
		size := int64(50)
		vol, err := createVolume(p, aws.String("snap-zxcv"), availabilityZone, size)
		Expect(err).To(BeNil())

		Expect(*vol.SnapshotID).To(Equal("snap-zxcv"))
		Expect(*vol.AvailabilityZone).To(Equal(*availabilityZone))
		Expect(*vol.Size).To(Equal(int64(50)))
		Expect(*vol.VolumeType).To(Equal(*GeneralVolumeType))
		Expect(*vol.State).To(Equal("available"))
	})
})

var _ = Describe("attachVolume", func() {
	It("attaches a volume", func() {
		p := &dummyVolumeProvider{}

		vol, err := createVolume(p, aws.String("snap-zxcv"), aws.String("us-west-1a"), 10)
		Expect(err).To(BeNil())

		instanceID := aws.String("i-1234")
		vol, err = attachVolume(p, vol.VolumeID, instanceID, aws.String("/dev/sda1"))
		Expect(err).To(BeNil())
		Expect(vol.Attachments).To(HaveLen(1))

		attachment := vol.Attachments[0]
		Expect(*attachment.InstanceID).To(Equal(*instanceID))
		Expect(*attachment.Device).To(Equal("/dev/sda1"))
		Expect(*attachment.State).To(Equal("attached"))
	})
})

var _ = Describe("createSnapshot", func() {
	It("creates a snapshot of given volume", func() {
		p := &dummyVolumeProvider{}

		volumeID := aws.String("vol-abcd")
		snapshot, err := createSnapshot(p, volumeID, aws.String("some description of snapshot"))
		Expect(err).To(BeNil())

		Expect(*snapshot.VolumeID).To(Equal(*volumeID))
	})
})
