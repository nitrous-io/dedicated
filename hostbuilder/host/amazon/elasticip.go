package amazon

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/ec2"
)

var (
	// wait X*(num attempts) milliseconds between each try to associate address
	AssociateBackoff = 3000

	// max attempts to associate if allocation not found
	AssociateMaxAttempts = 15
)

// interface of elastic IP provider
type ipProvider interface {
	Addresses(publicIps []string, allocationIds []string, filter *ec2.Filter) (resp *ec2.DescribeAddressesResp, err error)
	AllocateAddress(*ec2.AllocateAddress) (*ec2.AllocateAddressResp, error)
	AssociateAddress(*ec2.AssociateAddress) (*ec2.AssociateAddressResp, error)
	DisassociateAddress(id string) (resp *ec2.SimpleResp, err error)
	ReleaseAddress(id string) (resp *ec2.SimpleResp, err error)
	DisassociateAddressClassic(ip string) (resp *ec2.SimpleResp, err error)
	ReleasePublicAddress(ip string) (resp *ec2.SimpleResp, err error)
}

type elasticIP struct {
	PublicIP      string
	AllocationID  string
	AssociationID string
}

// assignAddress allocates an ElasticIP address to an instance
//
// More reference at:
// -http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_AssociateAddress.html
// -http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_AllocateAddress.html
func assignAddress(provider ipProvider, instanceID string, vpc bool) (*elasticIP, error) {
	// if the instance has VPC subnet ID, then we want to allocate a VPC
	// Elastic IP instead of a standard Elastic IP
	domain := "standard"
	if vpc {
		domain = "vpc"
	}

	allocateResp, err := provider.AllocateAddress(&ec2.AllocateAddress{Domain: domain})
	if err != nil {
		return nil, err
	}

	associateReq := &ec2.AssociateAddress{InstanceId: instanceID}
	if vpc {
		associateReq.AllocationId = allocateResp.AllocationId
	} else {
		associateReq.PublicIp = allocateResp.PublicIp
	}

	associationResp, err := tryAssociateAddress(provider, associateReq)
	if err != nil {
		var relerr error
		// if failed to associate the ElasticIp with the instance, we remove it
		if vpc {
			_, relerr = provider.ReleaseAddress(allocateResp.AllocationId)
		} else {
			_, relerr = provider.ReleasePublicAddress(allocateResp.PublicIp)
		}
		if relerr != nil {
			return nil, fmt.Errorf("error associating: %s; error releasing: %s", err, relerr)
		}
		return nil, err
	}

	return &elasticIP{
		PublicIP:      allocateResp.PublicIp,
		AllocationID:  allocateResp.AllocationId,
		AssociationID: associationResp.AssociationId,
	}, nil
}

// associateAddress associates an ElasticIP to an instance. The ElasticIP must
// be an allocated (i.e. previously acquired) IP address.
func associateAddress(provider ipProvider, ipAddress, instanceID string, vpc bool) (*elasticIP, error) {
	var allocationID string // Only set for VPC.

	req := &ec2.AssociateAddress{InstanceId: instanceID}
	if vpc {
		filter := ec2.NewFilter()
		filter.Add("public-ip", ipAddress)
		filter.Add("domain", "vpc")

		addresses, err := provider.Addresses(nil, nil, filter)
		if err != nil {
			return nil, err
		}
		if len(addresses.Addresses) == 0 {
			return nil, fmt.Errorf("could not associate Elastic IP %s that has not been previously allocated", ipAddress)
		}

		addr := addresses.Addresses[0]
		allocationID = addr.AllocationId

		req.AllocationId = allocationID
		req.AllowReassociation = false
	} else {
		req.PublicIp = ipAddress
	}

	associationResp, err := tryAssociateAddress(provider, req)
	if err != nil {
		return nil, err
	}

	return &elasticIP{
		PublicIP:      ipAddress,
		AllocationID:  allocationID,
		AssociationID: associationResp.AssociationId,
	}, nil
}

// releaseAddress release the allocated ElasticIP with the specified address.
//
// More reference at:
// - http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_ReleaseAddress.html
// - http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeAddresses.html
func releaseAddress(provider ipProvider, ipAddress string, vpc bool) error {
	filter := ec2.NewFilter()
	filter.Add("public-ip", ipAddress)
	if vpc {
		filter.Add("domain", "vpc")
	} else {
		filter.Add("domain", "standard")
	}

	addresses, err := provider.Addresses(nil, nil, filter)
	if err != nil {
		return err
	}
	// If we can't find the Elastic IP, consider the release to be successful.
	if len(addresses.Addresses) == 0 {
		return nil
	}

	addr := addresses.Addresses[0]
	if vpc {
		if addr.AssociationId != "" {
			if _, err := provider.DisassociateAddress(addr.AssociationId); err != nil {
				return err
			}
		}

		if addr.AllocationId != "" {
			if _, err := provider.ReleaseAddress(addr.AllocationId); err != nil {
				return err
			}
		}
	} else {
		if addr.PublicIp != "" {
			if _, err := provider.DisassociateAddressClassic(addr.PublicIp); err != nil {
				return err
			}

			if _, err := provider.ReleasePublicAddress(addr.PublicIp); err != nil {
				return err
			}
		}
	}
	return nil
}

// disassociateAddress disassociates an ElasticIP from the instance that it's
// associated with.
func disassociateAddress(provider ipProvider, ipAddress string, vpc bool) error {
	filter := ec2.NewFilter()
	filter.Add("public-ip", ipAddress)
	if vpc {
		filter.Add("domain", "vpc")
	} else {
		filter.Add("domain", "standard")
	}

	addresses, err := provider.Addresses(nil, nil, filter)
	if err != nil {
		return err
	}
	// If we can't find the Elastic IP, consider the release to be successful.
	if len(addresses.Addresses) == 0 {
		logrus.Warnf("disassociateAddress: attempted to disassociate EIP %s but it doesn't exist", ipAddress)
		return nil
	}

	addr := addresses.Addresses[0]
	if vpc {
		if addr.AssociationId != "" {
			// VPC.
			if _, err := provider.DisassociateAddress(addr.AssociationId); err != nil {
				return err
			}
		}
	} else {
		if addr.PublicIp != "" {
			// EC2 Classic.
			if _, err := provider.DisassociateAddressClassic(addr.PublicIp); err != nil {
				return err
			}
		}
	}

	return nil
}

// tryAssociateAddress attempts associate address multiple times with linear backoff,
// since there are situations when the allocated address is not ready yet
func tryAssociateAddress(provider ipProvider, opts *ec2.AssociateAddress) (*ec2.AssociateAddressResp, error) {
	var (
		resp *ec2.AssociateAddressResp
		err  error
	)
	for i := 0; i < AssociateMaxAttempts; i++ {
		time.Sleep(time.Duration(AssociateBackoff*i) * time.Millisecond)
		resp, err = provider.AssociateAddress(opts)
		switch assocErr := err.(type) {
		case nil:
			logrus.Debugf("tryAssociateAddress attempts before success: %d", i+1)
			return resp, nil
		case *ec2.Error:
			if assocErr.Code == "InvalidAllocationID.NotFound" {
				continue
			}
			return nil, err
		default:
			return nil, err
		}
	}
	return resp, err // persistent InvalidAllocationID.NotFound errors
}

// getAddresses returns the Elastic IP addresses associated with an EC2 instance.
func getAddresses(provider ipProvider, instanceID string) ([]ec2.Address, error) {
	filter := ec2.NewFilter()
	filter.Add("instance-id", instanceID)

	resp, err := provider.Addresses(nil, nil, filter)
	if err != nil {
		return nil, err
	}
	return resp.Addresses, nil
}
