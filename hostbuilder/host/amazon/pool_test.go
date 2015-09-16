package amazon

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("instancePoolConfig", func() {
	It("returns the instance pool config for the configured region and given instance type", func() {
		driverCfg := driverConfig{
			Env: "test",
			Regions: []regionConfig{
				{
					ID: "ap-southeast-42",
					InstancePool: []instancePoolConfig{
						{
							Type: "t2.micro",
							Max:  5,
						},
						{
							Type: "t2.small",
							Max:  3,
						},
					},
				},
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		cfg := driver.instancePoolConfig("t2.micro")
		Expect(cfg.Type).To(Equal("t2.micro"))
		Expect(cfg.Max).To(Equal(5))

		cfg = driver.instancePoolConfig("t2.small")
		Expect(cfg.Type).To(Equal("t2.small"))
		Expect(cfg.Max).To(Equal(3))
	})

	It("returns nil if the region does not have an instance pool configured for the given instance type", func() {
		driverCfg := driverConfig{
			Env: "test",
			Regions: []regionConfig{
				{
					ID: "ap-southeast-42",
					InstancePool: []instancePoolConfig{
						{
							Type: "t2.micro",
							Max:  5,
						},
					},
				},
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		// Unconfigured instance type.
		cfg := driver.instancePoolConfig("t2.small")
		Expect(cfg).To(BeNil())

		// Use a region that is not configured.
		driver.Config.RegionID = "unconfigured-region"

		cfg = driver.instancePoolConfig("t2.micro")
		Expect(cfg).To(BeNil())
	})
})

type fakeInstanceProvider struct {
	instances     []ec2.Instance
	instancesErr  error
	deleteTagsErr error
	createTagsErr error

	receivedIDs    []string
	receivedFilter *ec2.Filter
	receivedTags   []ec2.Tag
}

func (f *fakeInstanceProvider) Instances(ids []string, filter *ec2.Filter) (*ec2.InstancesResp, error) {
	f.receivedIDs = ids
	f.receivedFilter = filter

	resp := &ec2.InstancesResp{}
	for _, instance := range f.instances {
		reservation := ec2.Reservation{Instances: []ec2.Instance{instance}}
		resp.Reservations = append(resp.Reservations, reservation)
	}

	return resp, f.instancesErr
}

func (f *fakeInstanceProvider) DeleteTags(ids []string, tags []ec2.Tag) (*ec2.SimpleResp, error) {
	f.receivedIDs = ids
	f.receivedTags = tags

	return &ec2.SimpleResp{}, f.deleteTagsErr
}

func (f *fakeInstanceProvider) ModifyInstance(id string, options *ec2.ModifyInstance) (*ec2.ModifyInstanceResp, error) {
	return nil, errors.New("not implemented in test")
}

func (f *fakeInstanceProvider) CreateTags(ids []string, tags []ec2.Tag) (*ec2.SimpleResp, error) {
	f.receivedIDs = ids
	f.receivedTags = tags

	return &ec2.SimpleResp{}, f.createTagsErr
}

var _ = Describe("instancesInPool", func() {
	It("looks up instances using filters", func() {
		driver := &Host{
			driverConfig: driverConfig{Env: "test"},
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		fakeEC2 := &fakeInstanceProvider{
			instances: []ec2.Instance{
				{InstanceId: "i-abcde", InstanceType: "t2.micro"},
				{InstanceId: "i-bcdef", InstanceType: "t2.micro"},
			},
		}

		instances, err := driver.instancesInPool(fakeEC2, "t2.micro")
		Expect(err).To(BeNil())
		Expect(instances).To(HaveLen(2))

		Expect(fakeEC2.receivedIDs).To(BeNil())

		// Build up the expected filter. Since ec2.Filter does not expose a way
		// to get at its values, we have to compare them using deep equality.
		expectedFilter := ec2.NewFilter()
		expectedFilter.Add("instance-type", "t2.micro")
		expectedFilter.Add(fmt.Sprintf(`tag:%s`, poolTag), "true")
		expectedFilter.Add("tag:dedicated", "test")
		expectedFilter.Add("instance-state-name", "running")

		Expect(fakeEC2.receivedFilter).To(Equal(expectedFilter))
	})
})

var _ = Describe("poolSize", func() {
	It("returns the number of instances in the instance pool of the given instance type", func() {
		driver := &Host{
			driverConfig: driverConfig{Env: "test"},
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		fakeEC2 := &fakeInstanceProvider{
			instances: []ec2.Instance{
				{InstanceId: "i-abcde", InstanceType: "t2.micro"},
				{InstanceId: "i-bcdef", InstanceType: "t2.micro"},
			},
		}

		size, err := driver.poolSize(fakeEC2, "t2.micro")
		Expect(err).To(BeNil())
		Expect(size).To(Equal(2))
	})
})

var _ = Describe("isPoolFull", func() {
	It("returns true if there is no instance pool configured for the given instance type", func() {
		driverCfg := driverConfig{
			Env: "test",
			Regions: []regionConfig{
				{
					ID:           "ap-southeast-42",
					InstancePool: []instancePoolConfig{},
				},
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		fakeEC2 := &fakeInstanceProvider{}

		ok, err := driver.isPoolFull(fakeEC2, "t2.micro")
		Expect(err).To(BeNil())
		Expect(ok).To(BeTrue())
	})

	It("returns true if the number of instances in the pool is >= the configured Max", func() {
		driverCfg := driverConfig{
			Env: "test",
			Regions: []regionConfig{
				{
					ID: "ap-southeast-42",
					InstancePool: []instancePoolConfig{
						{
							Type: "t2.micro",
							Max:  2,
						},
					},
				},
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		fakeEC2 := &fakeInstanceProvider{
			instances: []ec2.Instance{
				{InstanceId: "i-abcde", InstanceType: "t2.micro"},
				{InstanceId: "i-bcdef", InstanceType: "t2.micro"},
			},
		}

		ok, err := driver.isPoolFull(fakeEC2, "t2.micro")
		Expect(err).To(BeNil())
		Expect(ok).To(BeTrue())

		// Change instance pool to have > Max instances.
		fakeEC2 = &fakeInstanceProvider{
			instances: []ec2.Instance{
				{InstanceId: "i-abcde", InstanceType: "t2.micro"},
				{InstanceId: "i-bcdef", InstanceType: "t2.micro"},
				{InstanceId: "i-cdefg", InstanceType: "t2.micro"},
			},
		}

		ok, err = driver.isPoolFull(fakeEC2, "t2.micro")
		Expect(err).To(BeNil())
		Expect(ok).To(BeTrue())
	})

	It("returns false if the nuumber of instances in the pool is < the configured Max", func() {
		driverCfg := driverConfig{
			Env: "test",
			Regions: []regionConfig{
				{
					ID: "ap-southeast-42",
					InstancePool: []instancePoolConfig{
						{
							Type: "t2.micro",
							Max:  2,
						},
					},
				},
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		fakeEC2 := &fakeInstanceProvider{
			instances: []ec2.Instance{
				{InstanceId: "i-abcde", InstanceType: "t2.micro"},
			},
		}

		ok, err := driver.isPoolFull(fakeEC2, "t2.micro")
		Expect(err).To(BeNil())
		Expect(ok).To(BeFalse())
	})
})

var _ = Describe("acquireInstance", func() {
	It("returns an error if there is no instance pool configured for the given instance type", func() {
		driver := &Host{
			driverConfig: driverConfig{Env: "test"},
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		instance, err := driver.acquireInstance(nil, "t2.micro")
		Expect(err).To(Equal(errPoolNotConfigured))
		Expect(instance).To(BeNil())
	})

	It("returns an error if the instance pool is empty", func() {
		driverCfg := driverConfig{
			Env: "test",
			Regions: []regionConfig{
				{
					ID: "ap-southeast-42",
					InstancePool: []instancePoolConfig{
						{
							Type: "t2.micro",
							Max:  5,
						},
					},
				},
			},
			Redis: redisConfig{
				Host: test.RedisHost,
				Port: test.RedisPort,
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		fakeEC2 := &fakeInstanceProvider{
			instances: []ec2.Instance{},
		}

		instance, err := driver.acquireInstance(fakeEC2, "t2.micro")
		Expect(err).To(Equal(errPoolEmpty))
		Expect(instance).To(BeNil())
	})

	It("acquires an instance from the pool", func() {
		driverCfg := driverConfig{
			Env: "test",
			Regions: []regionConfig{
				{
					ID: "ap-southeast-42",
					InstancePool: []instancePoolConfig{
						{
							Type: "t2.micro",
							Max:  5,
						},
					},
				},
			},
			Redis: redisConfig{
				Host: test.RedisHost,
				Port: test.RedisPort,
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		fakeEC2 := &fakeInstanceProvider{
			instances: []ec2.Instance{
				{InstanceId: "i-abcde", InstanceType: "t2.micro"},
				{InstanceId: "i-bcdef", InstanceType: "t2.micro"},
			},
		}

		instance, err := driver.acquireInstance(fakeEC2, "t2.micro")
		Expect(err).To(BeNil())
		Expect(instance.InstanceId).To(Equal("i-abcde"))
	})

	It("returns an error when it fails to get the instances that are in the pool", func() {
		driverCfg := driverConfig{
			Env: "test",
			Regions: []regionConfig{
				{
					ID: "ap-southeast-42",
					InstancePool: []instancePoolConfig{
						{
							Type: "t2.micro",
							Max:  5,
						},
					},
				},
			},
			Redis: redisConfig{
				Host: test.RedisHost,
				Port: test.RedisPort,
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		fakeEC2 := &fakeInstanceProvider{
			instancesErr: errors.New("EC2 API failwhaled"),
		}

		_, err := driver.acquireInstance(fakeEC2, "t2.micro")
		Expect(err).To(HaveOccurred())
	})

	It("sets the dedicated:pool tag of the acquired instance to the present time", func() {
		driverCfg := driverConfig{
			Env: "test",
			Regions: []regionConfig{
				{
					ID: "ap-southeast-42",
					InstancePool: []instancePoolConfig{
						{
							Type: "t2.micro",
							Max:  5,
						},
					},
				},
			},
			Redis: redisConfig{
				Host: test.RedisHost,
				Port: test.RedisPort,
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		fakeEC2 := &fakeInstanceProvider{
			instances: []ec2.Instance{
				{InstanceId: "i-abcde", InstanceType: "t2.micro"},
				{InstanceId: "i-bcdef", InstanceType: "t2.micro"},
			},
		}

		instance, err := driver.acquireInstance(fakeEC2, "t2.micro")
		Expect(err).To(BeNil())
		Expect(instance.InstanceId).To(Equal("i-abcde"))

		Expect(fakeEC2.receivedIDs).To(ConsistOf("i-abcde"))
		Expect(fakeEC2.receivedTags).To(HaveLen(1))
		t := fakeEC2.receivedTags[0]
		Expect(t.Key).To(Equal("dedicated:pool"))

		atimeInt, err := strconv.Atoi(t.Value)
		Expect(err).To(BeNil())
		Expect(atimeInt).To(BeNumerically("~", time.Now().Unix(), 5)) // 5 seconds
	})

	It("returns an error when it fails to delete tags from the acquired instance", func() {
		driverCfg := driverConfig{
			Env: "test",
			Regions: []regionConfig{
				{
					ID: "ap-southeast-42",
					InstancePool: []instancePoolConfig{
						{
							Type: "t2.micro",
							Max:  5,
						},
					},
				},
			},
			Redis: redisConfig{
				Host: test.RedisHost,
				Port: test.RedisPort,
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		fakeEC2 := &fakeInstanceProvider{
			createTagsErr: errors.New("EC2 tagging API timed out"),
			instances: []ec2.Instance{
				{InstanceId: "i-abcde", InstanceType: "t2.micro"},
				{InstanceId: "i-bcdef", InstanceType: "t2.micro"},
			},
		}

		_, err := driver.acquireInstance(fakeEC2, "t2.micro")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("releaseInstance", func() {
	It("tags and renames the instance", func() {
		driverCfg := driverConfig{
			Env: "test",
			Redis: redisConfig{
				Host: test.RedisHost,
				Port: test.RedisPort,
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
			instance:     &ec2.Instance{InstanceId: "i-abcde"},
		}

		fakeEC2 := &fakeInstanceProvider{}

		err := driver.releaseInstance(fakeEC2)
		Expect(err).To(BeNil())

		Expect(fakeEC2.receivedTags).To(ConsistOf(
			ec2.Tag{Key: poolTag, Value: "true"},
			ec2.Tag{Key: "Name", Value: "userhost-pool"},
		))
	})

	It("returns an error if it fails to tag the instance", func() {
		driverCfg := driverConfig{
			Env: "test",
			Redis: redisConfig{
				Host: test.RedisHost,
				Port: test.RedisPort,
			},
		}
		driver := &Host{
			driverConfig: driverCfg,
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
			instance:     &ec2.Instance{InstanceId: "i-abcde"},
		}

		fakeEC2 := &fakeInstanceProvider{
			createTagsErr: errors.New("EC2 tagging API timed out"),
		}

		err := driver.releaseInstance(fakeEC2)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("poolMutexName", func() {
	It("includes the configured region and given instance type", func() {
		driver := &Host{
			driverConfig: driverConfig{Env: "test"},
			Config:       &hostConfig{RegionID: "ap-southeast-42"},
		}

		name := driver.poolMutexName("t2.micro")
		Expect(name).To(ContainSubstring("t2.micro"))
		Expect(name).To(ContainSubstring("ap-southeast-42"))
	})
})
