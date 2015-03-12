package amazon

import "github.com/mitchellh/goamz/ec2"

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
	associationResp, err := provider.AssociateAddress(associateReq)
	if err != nil {
		// if failed to associate the ElasticIp with the instance, we remove it
		if vpc {
			provider.ReleaseAddress(allocateResp.AllocationId)
		} else {
			provider.ReleasePublicAddress(allocateResp.PublicIp)
		}
		return nil, err
	}
	return &elasticIP{
		PublicIP:      allocateResp.PublicIp,
		AllocationID:  allocateResp.AllocationId,
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
	// if we cant find the elastic IP, considered the release to be successful
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
