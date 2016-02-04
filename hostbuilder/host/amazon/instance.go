package amazon

import (
	"errors"
	"fmt"

	"github.com/mitchellh/goamz/ec2"
)

type instanceProvider interface {
	Instances(ids []string, filter *ec2.Filter) (*ec2.InstancesResp, error)
	ModifyInstance(id string, options *ec2.ModifyInstance) (*ec2.ModifyInstanceResp, error)
	CreateTags(ids []string, tags []ec2.Tag) (*ec2.SimpleResp, error)
	DeleteTags(ids []string, tags []ec2.Tag) (*ec2.SimpleResp, error)
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

func modifyInstanceType(provider instanceProvider, h *Host, instanceType string) error {
	// See http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_ModifyInstanceAttribute.html
	// for AWS API documentation.
	resp, err := provider.ModifyInstance(h.instance.InstanceId, &ec2.ModifyInstance{InstanceType: instanceType})
	if err != nil {
		return err
	}
	if !resp.Return {
		return errors.New("request to modify instance type failed")
	}
	return nil
}

func setTags(provider instanceProvider, h *Host, additionalTags []ec2.Tag) error {
	tags := []ec2.Tag{{"Name", h.Name()}}
	tags = append(tags, additionalTags...)
	if h.driverConfig.Env != "" {
		tags = append(tags, ec2.Tag{"dedicated", h.driverConfig.Env})
	}
	if h.driverConfig.HostVersion != "" {
		tags = append(tags, ec2.Tag{"dedicated:version", h.driverConfig.HostVersion})
	}
	if h.Config.Name == poolInstanceName {
		// it should be set to 'true' but at this point the instance should not
		// be considered available yet
		tags = append(tags, ec2.Tag{poolTag, "building"})
	}

	h.Printf("upserting tags on %s to %#v", h.instance.InstanceId, tags)
	_, err := provider.CreateTags([]string{h.instance.InstanceId}, tags)
	if err != nil {
		return fmt.Errorf("failed to set tags on instance %s", h.instance.InstanceId)
	}

	return nil
}
