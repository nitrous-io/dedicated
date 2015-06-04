package amazon

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type instanceProvider interface {
	DescribeInstances(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)
	CreateTags(*ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error)
}

func getInstance(provider instanceProvider, id string, h *Host) error {
	instancesResp, err := provider.DescribeInstances(&ec2.DescribeInstancesInput{
		InstanceIDs: []*string{aws.String(id)},
	})
	if err != nil {
		return err
	}
	if len(instancesResp.Reservations) == 0 || len(instancesResp.Reservations[0].Instances) == 0 {
		return ErrInstanceNotFound
	}
	h.instance = instancesResp.Reservations[0].Instances[0]
	return nil
}

func setTags(provider instanceProvider, h *Host) {
	tags := []*ec2.Tag{{
		Key:   aws.String("Name"),
		Value: aws.String(h.Name()),
	}}
	if h.env != "" {
		tags = append(tags, &ec2.Tag{
			Key:   aws.String("dedicated"),
			Value: aws.String(h.env),
		})
	}
	_, err := provider.CreateTags(
		&ec2.CreateTagsInput{
			Resources: []*string{h.instance.InstanceID},
			Tags:      tags,
		})
	if err != nil {
		h.Printf("Failed to set name for instance %s", h.instance.InstanceID)
	}
}
