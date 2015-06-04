package amazon

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestAmazon(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "host/amazon")
}

type fn struct {
	name string
	args []interface{}
}

// A baseError can be used as the root for errors satisfying the awserr.Error.
type baseError struct {
	code    string
	message string
	err     error
}

func (b *baseError) Error() string   { return fmt.Sprintf("%s: %s, %s", b.code, b.message, b.err) }
func (b *baseError) String() string  { return b.Error() }
func (b *baseError) Code() string    { return b.code }
func (b *baseError) Message() string { return b.message }
func (b *baseError) OrigErr() error  { return b.err }

type dummyIPProvider struct {
	addresses []*ec2.Address
	calls     []fn

	associateErr error
	releaseErr   error
}

func init() {
	AssociateBackoff = 1 // ms, reduce delay between AssociateAddress attempts
}

func (p *dummyIPProvider) DescribeAddresses(req *ec2.DescribeAddressesInput) (*ec2.DescribeAddressesOutput, error) {
	p.calls = append(p.calls, fn{"Addresses", []interface{}{req}})
	return &ec2.DescribeAddressesOutput{
		Addresses: p.addresses,
	}, nil
}

func (p *dummyIPProvider) AllocateAddress(req *ec2.AllocateAddressInput) (*ec2.AllocateAddressOutput, error) {
	p.calls = append(p.calls, fn{"AllocateAddress", []interface{}{req}})
	if len(p.addresses) == 0 {
		return nil, errors.New("an error message")
	}
	return &ec2.AllocateAddressOutput{
		PublicIP:     p.addresses[0].PublicIP,
		Domain:       p.addresses[0].Domain,
		AllocationID: p.addresses[0].AllocationID,
	}, nil
}

func (p *dummyIPProvider) AssociateAddress(req *ec2.AssociateAddressInput) (*ec2.AssociateAddressOutput, error) {
	p.calls = append(p.calls, fn{"AssociateAddress", []interface{}{req}})
	if p.associateErr != nil {
		return nil, p.associateErr
	}
	return &ec2.AssociateAddressOutput{
		AssociationID: aws.String("eip-assoc-001"),
	}, nil
}

func (p *dummyIPProvider) DisassociateAddress(req *ec2.DisassociateAddressInput) (*ec2.DisassociateAddressOutput, error) {
	p.calls = append(p.calls, fn{"DisassociateAddress", []interface{}{req}})
	return new(ec2.DisassociateAddressOutput), nil
}

func (p *dummyIPProvider) ReleaseAddress(req *ec2.ReleaseAddressInput) (*ec2.ReleaseAddressOutput, error) {
	p.calls = append(p.calls, fn{"ReleaseAddress", []interface{}{req}})
	if p.releaseErr != nil {
		return nil, p.releaseErr
	}
	return new(ec2.ReleaseAddressOutput), nil
}

var _ = Describe("assignAddress", func() {
	It("assigns a classic address", func() {
		p := &dummyIPProvider{
			addresses: []*ec2.Address{
				{
					PublicIP:     aws.String("10.0.0.1"),
					AllocationID: aws.String("eip-alloc-001"),
					Domain:       aws.String("standard"),
				},
			},
		}
		res, _ := assignAddress(p, aws.String("i-322"), false)
		Expect(res).To(Equal(&elasticIP{
			PublicIP:      "10.0.0.1",
			AllocationID:  "eip-alloc-001",
			AssociationID: "eip-assoc-001",
		}))

		Expect(p.calls[0].name).To(Equal("AllocateAddress"))
		req1 := p.calls[0].args[0].(*ec2.AllocateAddressInput)
		Expect(*req1.Domain).To(Equal("standard"))

		Expect(p.calls[1].name).To(Equal("AssociateAddress"))
		req2 := p.calls[1].args[0].(*ec2.AssociateAddressInput)
		// associate request can only have either public ip or allocation id
		Expect(req2.AllocationID).To(BeNil())
		Expect(*req2.PublicIP).To(Equal("10.0.0.1"))
	})

	It("rollbacks the classic address when it fails to associate the address", func() {
		p := &dummyIPProvider{
			addresses: []*ec2.Address{
				{
					PublicIP:     aws.String("10.0.0.1"),
					AllocationID: aws.String("eip-alloc-001"),
					Domain:       aws.String("standard"),
				},
			},
			associateErr: errors.New("an error message"),
		}

		_, err := assignAddress(p, aws.String("i-322"), false)
		Expect(err).To(HaveOccurred())
		Expect(p.calls[2].name).To(Equal("ReleaseAddress"))
		req := p.calls[2].args[0].(*ec2.ReleaseAddressInput)
		Expect(*req.PublicIP).To(Equal("10.0.0.1"))
	})

	It("assigns an vpc address", func() {
		p := &dummyIPProvider{
			addresses: []*ec2.Address{
				{
					PublicIP:     aws.String("10.0.0.1"),
					AllocationID: aws.String("eip-alloc-001"),
					Domain:       aws.String("vpc"),
				},
			},
		}
		res, _ := assignAddress(p, aws.String("i-322"), true)
		Expect(res).To(Equal(&elasticIP{
			PublicIP:      "10.0.0.1",
			AllocationID:  "eip-alloc-001",
			AssociationID: "eip-assoc-001",
		}))

		Expect(p.calls[0].name).To(Equal("AllocateAddress"))
		req1 := p.calls[0].args[0].(*ec2.AllocateAddressInput)
		Expect(*req1.Domain).To(Equal("vpc"))

		Expect(p.calls[1].name).To(Equal("AssociateAddress"))
		req2 := p.calls[1].args[0].(*ec2.AssociateAddressInput)
		// associate request can only have either public ip or allocation id
		Expect(*req2.AllocationID).To(Equal("eip-alloc-001"))
		Expect(req2.PublicIP).To(BeNil())

	})

	It("attempts multiple times when it cannot find allocated address", func() {
		p := &dummyIPProvider{
			addresses: []*ec2.Address{
				{
					PublicIP:     aws.String("10.0.0.1"),
					AllocationID: aws.String("eip-alloc-001"),
					Domain:       aws.String("vpc"),
				},
			},
			associateErr: &baseError{code: "InvalidAllocationID.NotFound"},
		}

		_, err := assignAddress(p, aws.String("i-322"), true)
		Expect(err).To(HaveOccurred())
		Expect(p.calls[0].name).To(Equal("AllocateAddress"))
		for i := 1; i <= AssociateMaxAttempts; i++ {
			Expect(p.calls[i].name).To(Equal("AssociateAddress"))
		}
		Expect(p.calls[AssociateMaxAttempts+1].name).To(Equal("ReleaseAddress"))
	})

	It("rollbacks the vpc address when it fails to associate the address", func() {
		p := &dummyIPProvider{
			addresses: []*ec2.Address{
				{
					PublicIP:     aws.String("10.0.0.1"),
					AllocationID: aws.String("eip-alloc-001"),
					Domain:       aws.String("vpc"),
				},
			},
			associateErr: errors.New("an error message"),
		}

		_, err := assignAddress(p, aws.String("i-322"), true)
		Expect(err).To(HaveOccurred())
		Expect(p.calls[2].name).To(Equal("ReleaseAddress"))
		req := p.calls[2].args[0].(*ec2.ReleaseAddressInput)
		Expect(*req.AllocationID).To(Equal("eip-alloc-001"))
	})

	It("includes release error if release fails on failure to associate address", func() {
		p := &dummyIPProvider{
			addresses: []*ec2.Address{
				{
					PublicIP:     aws.String("10.0.0.1"),
					AllocationID: aws.String("eip-alloc-001"),
					Domain:       aws.String("vpc"),
				},
			},
			associateErr: errors.New("association failed"),
			releaseErr:   errors.New("release failed"),
		}

		_, err := assignAddress(p, aws.String("i-322"), true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("association failed"))
		Expect(err.Error()).To(ContainSubstring("release failed"))
	})
})

var _ = Describe("releaseAddress", func() {
	It("succeeds if the address is not found", func() {
		p := &dummyIPProvider{
			addresses: []*ec2.Address{},
		}
		Expect(releaseAddress(p, aws.String("10.0.0.1"), false)).ToNot(HaveOccurred())
		Expect(len(p.calls)).To(Equal(1), "unexpected number of remote calls")
		Expect(p.calls[0].name).To(Equal("Addresses"))
	})

	It("releases classic address", func() {
		p := &dummyIPProvider{
			addresses: []*ec2.Address{
				{
					PublicIP:     aws.String("10.0.0.1"),
					AllocationID: aws.String("eip-001"),
					Domain:       aws.String("standard"),
					InstanceID:   aws.String("i-322"),
				},
			},
		}
		Expect(releaseAddress(p, aws.String("10.0.0.1"), false)).ToNot(HaveOccurred())
		Expect(p.calls[0].name).To(Equal("Addresses"))

		Expect(p.calls[1].name).To(Equal("DisassociateAddress"))
		req1 := p.calls[1].args[0].(*ec2.DisassociateAddressInput)
		Expect(*req1.PublicIP).To(Equal("10.0.0.1"))

		Expect(p.calls[2].name).To(Equal("ReleaseAddress"))
		req2 := p.calls[2].args[0].(*ec2.ReleaseAddressInput)
		Expect(*req2.PublicIP).To(Equal("10.0.0.1"))
	})

	It("releases vpc address", func() {
		p := &dummyIPProvider{
			addresses: []*ec2.Address{
				{
					PublicIP:      aws.String("10.0.0.1"),
					AllocationID:  aws.String("eip-001"),
					AssociationID: aws.String("eip-asso-001"),
					Domain:        aws.String("vpc"),
					InstanceID:    aws.String("i-322"),
				},
			},
		}
		Expect(releaseAddress(p, aws.String("10.0.0.1"), true)).ToNot(HaveOccurred(), "failed to release address")
		Expect(p.calls[0].name).To(Equal("Addresses"))
		Expect(p.calls[1].name).To(Equal("DisassociateAddress"))

		req1 := p.calls[1].args[0].(*ec2.DisassociateAddressInput)
		Expect(*req1.AssociationID).To(Equal("eip-asso-001"))

		Expect(p.calls[2].name).To(Equal("ReleaseAddress"))
		req2 := p.calls[2].args[0].(*ec2.ReleaseAddressInput)
		Expect(*req2.AllocationID).To(Equal("eip-001"))
	})

	It("does not try to dissassociate VPC IP when it's not attached", func() {
		p := &dummyIPProvider{
			addresses: []*ec2.Address{
				{
					PublicIP:     aws.String("10.0.0.1"),
					AllocationID: aws.String("eip-001"),
					Domain:       aws.String("vpc"),
					InstanceID:   aws.String("i-322"),
				},
			},
		}
		Expect(releaseAddress(p, aws.String("10.0.0.1"), true)).ToNot(HaveOccurred(), "failed to release address")
		Expect(p.calls[0].name).To(Equal("Addresses"))
		Expect(p.calls[1].name).To(Equal("ReleaseAddress"))

		req1 := p.calls[1].args[0].(*ec2.ReleaseAddressInput)
		Expect(*req1.AllocationID).To(Equal("eip-001"))
	})
})

var _ = Describe("getAddresses", func() {
	It("returns the addresses associated with an EC2 instance", func() {
		instanceID := "i-acafe"
		p := &dummyIPProvider{
			addresses: []*ec2.Address{
				{
					AllocationID: aws.String("eip-001"),
					PublicIP:     aws.String("10.0.0.1"),
					Domain:       aws.String("standard"),
					InstanceID:   aws.String(instanceID),
				},
			},
		}

		addresses, err := getAddresses(p, aws.String(instanceID))
		Expect(err).To(BeNil())

		Expect(addresses).To(HaveLen(1))
		Expect(*addresses[0].AllocationID).To(Equal("eip-001"))
		Expect(*addresses[0].PublicIP).To(Equal("10.0.0.1"))

		Expect(p.calls).To(HaveLen(1))
		Expect(p.calls[0].name).To(Equal("Addresses"))

		req1 := p.calls[0].args[0].(*ec2.DescribeAddressesInput)
		Expect(*req1.Filters[0].Name).To(Equal("instance-id"))
		Expect(*req1.Filters[0].Values[0]).To(Equal(instanceID))
	})
})
