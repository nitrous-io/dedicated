package amazon

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
)

var (
	// wait X*(num attempts) milliseconds between each try to associate address
	AssociateBackoff = 3000

	// max attempts to associate if allocation not found
	AssociateMaxAttempts = 15
)

// interface of elastic IP provider
type ipProvider interface {
	DescribeAddresses(*ec2.DescribeAddressesInput) (resp *ec2.DescribeAddressesOutput, err error)
	AllocateAddress(*ec2.AllocateAddressInput) (*ec2.AllocateAddressOutput, error)
	AssociateAddress(*ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error)
	DisassociateAddress(*ec2.DisassociateAddressInput) (resp *ec2.DisassociateAddressOutput, err error)
	ReleaseAddress(*ec2.ReleaseAddressInput) (resp *ec2.ReleaseAddressOutput, err error)
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
func assignAddress(provider ipProvider, instanceID *string, vpc bool) (*elasticIP, error) {
	if instanceID == nil {
		return nil, fmt.Errorf("amazon: assignAddresss invalid arguments")
	}
	// if the instance has VPC subnet ID, then we want to allocate a VPC
	// Elastic IP instead of a standard Elastic IP
	domain := aws.String("standard")
	if vpc {
		domain = aws.String("vpc")
	}

	allocateResp, err := provider.AllocateAddress(&ec2.AllocateAddressInput{
		Domain: domain,
	})
	if err != nil {
		return nil, err
	}

	associateReq := &ec2.AssociateAddressInput{InstanceID: instanceID}
	if vpc {
		associateReq.AllocationID = allocateResp.AllocationID
	} else {
		associateReq.PublicIP = allocateResp.PublicIP
	}

	associationResp, err := tryAssociateAddress(provider, associateReq)
	if err != nil {
		releaseInput := &ec2.ReleaseAddressInput{}
		// if failed to associate the ElasticIp with the instance, we remove it
		if vpc {
			releaseInput.AllocationID = allocateResp.AllocationID
		} else {
			releaseInput.PublicIP = allocateResp.PublicIP
		}
		if _, relerr := provider.ReleaseAddress(releaseInput); relerr != nil {
			return nil, fmt.Errorf("error associating: %s; error releasing: %s", err, relerr)
		}
		return nil, err
	}
	if allocateResp.PublicIP == nil ||
		allocateResp.AllocationID == nil ||
		associationResp.AssociationID == nil {
		return nil, fmt.Errorf("amazon: invalid allocate/associate response")
	}
	return &elasticIP{
		PublicIP:      *allocateResp.PublicIP,
		AllocationID:  *allocateResp.AllocationID,
		AssociationID: *associationResp.AssociationID,
	}, nil
}

// releaseAddress release the allocated ElasticIP with the specified address.
//
// More reference at:
// - http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_ReleaseAddress.html
// - http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeAddresses.html
func releaseAddress(provider ipProvider, ipAddress *string, vpc bool) error {
	domain := aws.String("standard")
	if vpc {
		domain = aws.String("vpc")
	}
	addresses, err := provider.DescribeAddresses(&ec2.DescribeAddressesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("public-ip"),
				Values: []*string{ipAddress},
			},
			{
				Name:   aws.String("domain"),
				Values: []*string{domain},
			},
		},
	})
	if err != nil {
		return err
	}
	// if we can't find the Elastic IP, consider the release to be successful.
	if len(addresses.Addresses) == 0 {
		return nil
	}

	addr := addresses.Addresses[0]
	if vpc {
		// disassociate the address if it's associated
		if addr.AssociationID != nil {
			if _, err := provider.DisassociateAddress(&ec2.DisassociateAddressInput{
				AssociationID: addr.AssociationID,
			}); err != nil {
				return err
			}
		}
		// release the address if it's allocated
		if addr.AllocationID != nil {
			if _, err := provider.ReleaseAddress(&ec2.ReleaseAddressInput{
				AllocationID: addr.AllocationID,
			}); err != nil {
				return err
			}
		}
	} else {
		// disassociate and release the public IP if it exists
		if addr.PublicIP != nil {
			if _, err := provider.DisassociateAddress(&ec2.DisassociateAddressInput{
				PublicIP: addr.PublicIP,
			}); err != nil {
				return err
			}
			if _, err := provider.ReleaseAddress(&ec2.ReleaseAddressInput{
				PublicIP: addr.PublicIP,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// tryAssociateAddress attempts associate address multiple times with linear backoff,
// since there are situations when the allocated address is not ready yet
func tryAssociateAddress(provider ipProvider, opts *ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error) {
	var (
		resp *ec2.AssociateAddressOutput
		err  error
	)
	for i := 0; i < AssociateMaxAttempts; i++ {
		time.Sleep(time.Duration(AssociateBackoff*i) * time.Millisecond)
		resp, err = provider.AssociateAddress(opts)
		switch assocErr := err.(type) {
		case nil:
			logrus.Debugf("tryAssociateAddress attempts before success: %d", i+1)
			return resp, nil
		case awserr.Error:
			if assocErr.Code() == "InvalidAllocationID.NotFound" {
				continue
			}
			return nil, err
		default:
			return nil, err
		}
	}
	return resp, err // persistent InvalidAllocationID.NotFound errors
}

// getAddresses returns the addresses associated with an EC2 instance.
func getAddresses(provider ipProvider, instanceID *string) ([]*ec2.Address, error) {
	if instanceID == nil {
		return nil, fmt.Errorf("amazon: getAddresses invalid arguments")
	}
	addresses, err := provider.DescribeAddresses(&ec2.DescribeAddressesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("instance-id"),
				Values: []*string{instanceID},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return addresses.Addresses, nil
}
