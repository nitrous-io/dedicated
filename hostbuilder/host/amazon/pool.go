package amazon

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/ec2"
)

/*
 * Instance pool-related code.
 */

var poolTag = "dedicated:pool"

type instancePoolConfig struct {
	Type string
	Max  int
}

// instancePool returns the configuration for the instance pool of the given
// instance type.
func (h *Host) instancePool(instancetype string) *instancePoolConfig {
	for _, r := range h.driverConfig.Regions {
		if r.ID != h.Config.RegionID {
			continue
		}

		pools := r.InstancePool
		for _, p := range pools {
			if p.Type != instancetype {
				continue
			}

			return &instancePoolConfig{Type: p.Type, Max: p.Max}
		}
	}

	return nil
}

// poolSize returns the number of running instances currently in the instance
// pool of the given instance type.
func (h *Host) poolSize(instancetype string) (int, error) {
	filter := ec2.NewFilter()
	filter.Add("instance-type", instancetype)
	filter.Add(fmt.Sprintf(`tag:%s`, poolTag), "true")
	filter.Add("instance-state-name", "running")

	// TODO We should actually paginate through requests, but we are just gonna
	// rely on the API returning at least 1000 results, which is bigger than the
	// size of any instance pool we want to configure, for now.
	resp, err := h.provider.Instances(nil, filter)
	if err != nil {
		return 0, err
	}

	if len(resp.Reservations) == 0 {
		return 0, nil
	}

	var count int
	for _, r := range resp.Reservations {
		count = count + len(r.Instances)
	}

	return count, nil
}

// poolFull returns true when the instance pool for the given instance type is
// full.
func (h *Host) isPoolFull(instancetype string) (bool, error) {
	pool := h.instancePool(instancetype)
	if pool == nil {
		// A non-existent pool is considered full (i.e. doesn't need any more
		// new instances).
		return true, nil
	}

	poolSize, err := h.poolSize(instancetype)
	if err != nil {
		return false, err
	}

	logrus.Infof("amazon: current %s pool size: %d, configured: %d", instancetype, poolSize, pool.Max)

	return (poolSize >= pool.Max), nil
}

// releaseInstance releases an instance to the pool of available instances. We
// indicate that an instance is in a pool by adding a "dedicated:pool=true" tag.
func (h *Host) releaseInstance() error {
	tags := []ec2.Tag{
		{Key: poolTag, Value: "true"},
		{Key: "Name", Value: "userhost-pool"},
	}
	if _, err := h.provider.CreateTags([]string{h.instance.InstanceId}, tags); err != nil {
		return err
	}

	return nil
}
