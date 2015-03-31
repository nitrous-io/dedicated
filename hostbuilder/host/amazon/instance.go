package amazon

import (
	"github.com/mitchellh/goamz/ec2"
)

type instanceProvider interface {
	Instances(ids []string, filter *ec2.Filter) (*ec2.InstancesResp, error)
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

func setTag(provider instanceProvider, h *Host) error {
	return nil
}
