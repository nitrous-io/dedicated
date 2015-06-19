package amazon

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type dummyInstanceProvider struct {
	instances []*ec2.Instance
	calls     []fn
}

func (p *dummyInstanceProvider) DescribeInstances(req *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error) {
	p.calls = append(p.calls, fn{"Instances", []interface{}{req}})
	instances := []*ec2.Instance{}
	for _, i := range p.instances {
		for _, id := range req.InstanceIDs {
			if *i.InstanceID == *id {
				instances = append(instances, i)
			}
		}
	}
	return &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			{Instances: instances},
		},
	}, nil
}

func (p *dummyInstanceProvider) CreateTags(req *ec2.CreateTagsInput) (*ec2.CreateTagsOutput, error) {
	p.calls = append(p.calls, fn{"CreateTags", []interface{}{req}})
	return &ec2.CreateTagsOutput{}, nil
}

var _ = Describe("getInstance", func() {
	It("returns the matched instance", func() {
		h := &Host{}
		p := &dummyInstanceProvider{
			instances: []*ec2.Instance{
				{InstanceID: aws.String("i-1234"), InstanceType: aws.String("m2.micro")},
				{InstanceID: aws.String("i-2345"), InstanceType: aws.String("m1.xlarge")},
			},
		}

		_ = getInstance(p, "i-2345", h)
		Expect(*h.instance.InstanceType).To(Equal("m1.xlarge"))
		req1 := p.calls[0].args[0].(*ec2.DescribeInstancesInput)
		Expect(*req1.InstanceIDs[0]).To(Equal("i-2345"))
	})

	It("returns error when there's no match", func() {
		h := &Host{}
		p := &dummyInstanceProvider{
			instances: []*ec2.Instance{
				{InstanceID: aws.String("i-1234"), InstanceType: aws.String("m2.micro")},
				{InstanceID: aws.String("i-2345"), InstanceType: aws.String("m1.xlarge")},
			},
		}

		err := getInstance(p, "i-23456", h)
		Expect(err).To(Equal(ErrInstanceNotFound))
		Expect(h.instance).To(BeNil())
	})
})

var _ = Describe("setTags", func() {
	It("set name and dedicated attribute", func() {
		var h *Host
		p := &dummyInstanceProvider{}
		h = &Host{
			Config:   &HostConfig{Name: "test-host"},
			instance: &ec2.Instance{InstanceID: aws.String("i-1234")},
		}
		setTags(p, h)

		req1 := p.calls[0].args[0].(*ec2.CreateTagsInput)
		Expect(req1.Resources).To(HaveLen(1))
		Expect(*req1.Resources[0]).To(Equal("i-1234"))
		Expect(req1.Tags).To(HaveLen(1))
		Expect(*req1.Tags[0].Key).To(Equal("Name"))
		Expect(*req1.Tags[0].Value).To(Equal("test-host"))

		p = &dummyInstanceProvider{}
		h = &Host{
			Config:   &HostConfig{Name: "test-{{env}}-host"},
			instance: &ec2.Instance{InstanceID: aws.String("i-1234")},
			env:      "staging",
		}
		setTags(p, h)
		req2 := p.calls[0].args[0].(*ec2.CreateTagsInput)
		Expect(req2.Tags).To(HaveLen(2))
		Expect(*req2.Tags[0].Key).To(Equal("Name"))
		Expect(*req2.Tags[0].Value).To(Equal("test-staging-host"))
		Expect(*req2.Tags[1].Key).To(Equal("dedicated"))
		Expect(*req2.Tags[1].Value).To(Equal("staging"))
	})
})
