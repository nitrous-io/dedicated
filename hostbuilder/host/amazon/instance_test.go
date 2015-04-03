package amazon

import (
	"github.com/mitchellh/goamz/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type dummyInstanceProvider struct {
	instances []ec2.Instance
	calls     []fn
}

func (p *dummyInstanceProvider) Instances(ids []string, filter *ec2.Filter) (resp *ec2.InstancesResp, err error) {
	p.calls = append(p.calls, fn{"Instances", []interface{}{ids, filter}})
	instances := []ec2.Instance{}
	for _, i := range p.instances {
		for _, id := range ids {
			if i.InstanceId == id {
				instances = append(instances, i)
			}
		}
	}
	return &ec2.InstancesResp{
		Reservations: []ec2.Reservation{
			{Instances: instances},
		},
	}, nil
}

func (p *dummyInstanceProvider) CreateTags(ids []string, tags []ec2.Tag) (*ec2.SimpleResp, error) {
	p.calls = append(p.calls, fn{"CreateTags", []interface{}{ids, tags}})
	return &ec2.SimpleResp{}, nil
}

var _ = Describe("getInstance", func() {
	It("returns the matched instance", func() {
		h := &Host{}
		p := &dummyInstanceProvider{
			instances: []ec2.Instance{
				{InstanceId: "i-1234", InstanceType: "m2.micro"},
				{InstanceId: "i-2345", InstanceType: "m1.xlarge"},
			},
		}

		_ = getInstance(p, "i-2345", h)
		Expect(h.instance.InstanceType).To(Equal("m1.xlarge"))
		Expect(p.calls[0].args[0].([]string)[0]).To(Equal("i-2345"))
	})

	It("returns error when there's no match", func() {
		h := &Host{}
		p := &dummyInstanceProvider{
			instances: []ec2.Instance{
				{InstanceId: "i-1234", InstanceType: "m2.micro"},
				{InstanceId: "i-2345", InstanceType: "m1.xlarge"},
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
			instance: &ec2.Instance{InstanceId: "i-1234"},
		}
		setTags(p, h)

		Expect(p.calls[0].args[0].([]string)).To(Equal([]string{"i-1234"}))
		Expect(p.calls[0].args[1].([]ec2.Tag)).To(Equal([]ec2.Tag{
			{"Name", "test-host"},
		}))

		p = &dummyInstanceProvider{}
		h = &Host{
			Config:   &HostConfig{Name: "test-host"},
			instance: &ec2.Instance{InstanceId: "i-1234"},
			env:      "staging",
		}
		setTags(p, h)
		Expect(p.calls[0].args[1].([]ec2.Tag)).To(Equal([]ec2.Tag{
			{"Name", "test-host"},
			{"dedicated", "staging"},
		}))
	})
})
