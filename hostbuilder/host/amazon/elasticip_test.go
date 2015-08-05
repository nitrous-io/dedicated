package amazon

import (
	"errors"

	"github.com/mitchellh/goamz/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type fn struct {
	name string
	args []interface{}
}

type dummyIPProvider struct {
	addresses []ec2.Address
	calls     []fn

	associateErr error
	releaseErr   error
}

func init() {
	AssociateBackoff = 1 // ms, reduce delay between AssociateAddress attempts
}

func (p *dummyIPProvider) Addresses(publicIps []string, allocationIds []string, filter *ec2.Filter) (resp *ec2.DescribeAddressesResp, err error) {
	p.calls = append(p.calls, fn{"Addresses", []interface{}{publicIps, allocationIds, filter}})
	return &ec2.DescribeAddressesResp{
		Addresses: p.addresses,
	}, nil
}

func (p *dummyIPProvider) AllocateAddress(req *ec2.AllocateAddress) (*ec2.AllocateAddressResp, error) {
	p.calls = append(p.calls, fn{"AllocateAddress", []interface{}{req}})
	if len(p.addresses) == 0 {
		return nil, errors.New("an error message")
	}
	return &ec2.AllocateAddressResp{
		RequestId:    "eip-req-001",
		PublicIp:     p.addresses[0].PublicIp,
		Domain:       p.addresses[0].Domain,
		AllocationId: p.addresses[0].AllocationId,
	}, nil
}

func (p *dummyIPProvider) AssociateAddress(req *ec2.AssociateAddress) (*ec2.AssociateAddressResp, error) {
	p.calls = append(p.calls, fn{"AssociateAddress", []interface{}{req}})
	if p.associateErr != nil {
		return nil, p.associateErr
	}
	return &ec2.AssociateAddressResp{
		AssociationId: "eip-assoc-001",
	}, nil
}

func (p *dummyIPProvider) DisassociateAddress(id string) (resp *ec2.SimpleResp, err error) {
	p.calls = append(p.calls, fn{"DisassociateAddress", []interface{}{id}})
	return new(ec2.SimpleResp), nil
}

func (p *dummyIPProvider) ReleaseAddress(id string) (resp *ec2.SimpleResp, err error) {
	p.calls = append(p.calls, fn{"ReleaseAddress", []interface{}{id}})
	if p.releaseErr != nil {
		return nil, p.releaseErr
	}
	return new(ec2.SimpleResp), nil
}

func (p *dummyIPProvider) DisassociateAddressClassic(ip string) (resp *ec2.SimpleResp, err error) {
	p.calls = append(p.calls, fn{"DisassociateAddressClassic", []interface{}{ip}})
	return new(ec2.SimpleResp), nil
}

func (p *dummyIPProvider) ReleasePublicAddress(ip string) (resp *ec2.SimpleResp, err error) {
	p.calls = append(p.calls, fn{"ReleasePublicAddress", []interface{}{ip}})
	return new(ec2.SimpleResp), nil
}

var _ = Describe("assignAddress", func() {
	It("assigns a classic address", func() {
		p := &dummyIPProvider{
			addresses: []ec2.Address{
				ec2.Address{
					PublicIp:     "10.0.0.1",
					AllocationId: "eip-alloc-001",
					Domain:       "standard",
				},
			},
		}
		res, _ := assignAddress(p, "i-322", false)
		Expect(res).To(Equal(&elasticIP{
			PublicIP:      "10.0.0.1",
			AllocationID:  "eip-alloc-001",
			AssociationID: "eip-assoc-001",
		}))

		Expect(p.calls[0].name).To(Equal("AllocateAddress"))
		Expect(p.calls[0].args[0].(*ec2.AllocateAddress).Domain).To(Equal("standard"))
		Expect(p.calls[1].name).To(Equal("AssociateAddress"))
		// associate request can only have either public ip or allocation id
		Expect(p.calls[1].args[0].(*ec2.AssociateAddress).AllocationId).To(BeEmpty())
		Expect(p.calls[1].args[0].(*ec2.AssociateAddress).PublicIp).To(Equal("10.0.0.1"))
	})

	It("rollbacks the classic address when it fails to associate the address", func() {
		p := &dummyIPProvider{
			addresses: []ec2.Address{
				ec2.Address{
					PublicIp:     "10.0.0.1",
					AllocationId: "eip-alloc-001",
					Domain:       "standard",
				},
			},
			associateErr: errors.New("an error message"),
		}

		_, err := assignAddress(p, "i-322", false)
		Expect(err).To(HaveOccurred())
		Expect(p.calls[2].name).To(Equal("ReleasePublicAddress"))
		Expect(p.calls[2].args[0].(string)).To(Equal("10.0.0.1"))
	})

	It("assigns an vpc address", func() {
		p := &dummyIPProvider{
			addresses: []ec2.Address{
				ec2.Address{
					PublicIp:     "10.0.0.1",
					AllocationId: "eip-alloc-001",
					Domain:       "vpc",
				},
			},
		}
		res, _ := assignAddress(p, "i-322", true)
		Expect(res).To(Equal(&elasticIP{
			PublicIP:      "10.0.0.1",
			AllocationID:  "eip-alloc-001",
			AssociationID: "eip-assoc-001",
		}))

		Expect(p.calls[0].name).To(Equal("AllocateAddress"))
		Expect(p.calls[0].args[0].(*ec2.AllocateAddress).Domain).To(Equal("vpc"))
		Expect(p.calls[1].name).To(Equal("AssociateAddress"))
		// associate request can only have either public ip or allocation id
		Expect(p.calls[1].args[0].(*ec2.AssociateAddress).AllocationId).To(Equal("eip-alloc-001"))
		Expect(p.calls[1].args[0].(*ec2.AssociateAddress).PublicIp).To(BeEmpty())

	})

	It("attempts multiple times when it cannot find allocated address", func() {
		p := &dummyIPProvider{
			addresses: []ec2.Address{
				ec2.Address{
					PublicIp:     "10.0.0.1",
					AllocationId: "eip-alloc-001",
					Domain:       "vpc",
				},
			},
			associateErr: &ec2.Error{Code: "InvalidAllocationID.NotFound"},
		}

		_, err := assignAddress(p, "i-322", true)
		Expect(err).To(HaveOccurred())
		Expect(p.calls[0].name).To(Equal("AllocateAddress"))
		for i := 1; i <= AssociateMaxAttempts; i++ {
			Expect(p.calls[i].name).To(Equal("AssociateAddress"))
		}
		Expect(p.calls[AssociateMaxAttempts+1].name).To(Equal("ReleaseAddress"))
	})

	It("rollbacks the vpc address when it fails to associate the address", func() {
		p := &dummyIPProvider{
			addresses: []ec2.Address{
				ec2.Address{
					PublicIp:     "10.0.0.1",
					AllocationId: "eip-alloc-001",
					Domain:       "vpc",
				},
			},
			associateErr: errors.New("an error message"),
		}

		_, err := assignAddress(p, "i-322", true)
		Expect(err).To(HaveOccurred())
		Expect(p.calls[2].name).To(Equal("ReleaseAddress"))
		Expect(p.calls[2].args[0].(string)).To(Equal("eip-alloc-001"))
	})

	It("includes release error if release fails on failure to associate address", func() {
		p := &dummyIPProvider{
			addresses: []ec2.Address{
				ec2.Address{
					PublicIp:     "10.0.0.1",
					AllocationId: "eip-alloc-001",
					Domain:       "vpc",
				},
			},
			associateErr: errors.New("association failed"),
			releaseErr:   errors.New("release failed"),
		}

		_, err := assignAddress(p, "i-322", true)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("association failed"))
		Expect(err.Error()).To(ContainSubstring("release failed"))
	})
})

var _ = Describe("releaseAddress", func() {
	It("succeeds if the address is not found", func() {
		p := &dummyIPProvider{
			addresses: []ec2.Address{},
		}
		Expect(releaseAddress(p, "10.0.0.1", false)).ToNot(HaveOccurred())
		Expect(len(p.calls)).To(Equal(1), "unexpected number of remote calls")
		Expect(p.calls[0].name).To(Equal("Addresses"))
	})

	It("releases classic address", func() {
		p := &dummyIPProvider{
			addresses: []ec2.Address{
				ec2.Address{
					PublicIp:     "10.0.0.1",
					AllocationId: "eip-001",
					Domain:       "standard",
					InstanceId:   "i-322",
				},
			},
		}
		Expect(releaseAddress(p, "10.0.0.1", false)).ToNot(HaveOccurred())
		Expect(p.calls[0].name).To(Equal("Addresses"))
		Expect(p.calls[1].name).To(Equal("DisassociateAddressClassic"))
		Expect(p.calls[1].args[0].(string)).To(Equal("10.0.0.1"))
		Expect(p.calls[2].name).To(Equal("ReleasePublicAddress"))
		Expect(p.calls[2].args[0].(string)).To(Equal("10.0.0.1"))
	})

	It("releases vpc address", func() {
		p := &dummyIPProvider{
			addresses: []ec2.Address{
				ec2.Address{
					PublicIp:      "10.0.0.1",
					AllocationId:  "eip-001",
					AssociationId: "eip-asso-001",
					Domain:        "vpc",
					InstanceId:    "i-322",
				},
			},
		}
		Expect(releaseAddress(p, "10.0.0.1", true)).ToNot(HaveOccurred(), "failed to release address")
		Expect(p.calls[0].name).To(Equal("Addresses"))
		Expect(p.calls[1].name).To(Equal("DisassociateAddress"))
		Expect(p.calls[1].args[0].(string)).To(Equal("eip-asso-001"))
		Expect(p.calls[2].name).To(Equal("ReleaseAddress"))
		Expect(p.calls[2].args[0].(string)).To(Equal("eip-001"))
	})

	It("does not try to dissassociate VPC IP when it's not attached", func() {
		p := &dummyIPProvider{
			addresses: []ec2.Address{
				ec2.Address{
					PublicIp:     "10.0.0.1",
					AllocationId: "eip-001",
					Domain:       "vpc",
					InstanceId:   "i-322",
				},
			},
		}
		Expect(releaseAddress(p, "10.0.0.1", true)).ToNot(HaveOccurred(), "failed to release address")
		Expect(p.calls[0].name).To(Equal("Addresses"))
		Expect(p.calls[1].name).To(Equal("ReleaseAddress"))
		Expect(p.calls[1].args[0].(string)).To(Equal("eip-001"))
	})
})

var _ = Describe("getAddresses", func() {
	It("returns the addresses associated with an EC2 instance", func() {
		instanceID := "i-acafe"
		p := &dummyIPProvider{
			addresses: []ec2.Address{
				{
					AllocationId: "eip-001",
					PublicIp:     "10.0.0.1",
					Domain:       "standard",
					InstanceId:   instanceID,
				},
			},
		}

		addresses, err := getAddresses(p, instanceID)
		Expect(err).To(BeNil())

		Expect(addresses).To(HaveLen(1))
		Expect(addresses[0].AllocationId).To(Equal("eip-001"))
		Expect(addresses[0].PublicIp).To(Equal("10.0.0.1"))

		Expect(p.calls).To(HaveLen(1))
		Expect(p.calls[0].name).To(Equal("Addresses"))
		filter := ec2.NewFilter()
		filter.Add("instance-id", instanceID)
		Expect(p.calls[0].args[2]).To(Equal(filter))
	})
})
