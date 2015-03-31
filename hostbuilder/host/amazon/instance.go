package amazon

import (
	"github.com/mitchellh/goamz/ec2"
)

type instanceProvider interface {
	Instances(ids []string, filter *ec2.Filter) (*ec2.InstancesResp, error)
	CreateTags(ids []string, tags []ec2.Tag) (*ec2.SimpleResp, error)
}

func getInstance(provider instanceProvider, id string, h *Host) error {
	instancesResp, err := provider.Instances([]string{id}, nil)
	if err != nil {
		return err
	}
	if len(instancesResp.Reservations) == 0 || len(instancesResp.Reservations[0].Instances) == 0 {
		return ErrInstanceNotFound
	}
	h.instance = &instancesResp.Reservations[0].Instances[0]
	return nil
}

func setTags(provider instanceProvider, h *Host) {
	tags := []ec2.Tag{{"Name", h.Config.Name}}
	if h.env != "" {
		tags = append(tags, ec2.Tag{"dedicated", h.env})
	}
	_, err := provider.CreateTags([]string{h.instance.InstanceId}, tags)
	if err != nil {
		h.Printf("Failed to set name for instance %s", h.instance.InstanceId)
	}
}
