package amazon

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/pkg/distsync"
)

/*
 * Instance pool-related code.
 */

var poolTag = "dedicated:pool"

type instancePoolConfig struct {
	Type string
	Max  int
}

// instancePoolConfig returns the configuration for the instance pool of the
// given instance type.
func (h *Host) instancePoolConfig(instancetype string) *instancePoolConfig {
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

// instancesInPool returns the instances in the pool for the given instance
// type. Instances should be running and tagged with "dedicated:pool=true".
func (h *Host) instancesInPool(instancetype string) ([]*ec2.Instance, error) {
	filter := ec2.NewFilter()
	filter.Add("instance-type", instancetype)
	filter.Add(fmt.Sprintf(`tag:%s`, poolTag), "true")
	filter.Add(`tag:dedicated`, h.driverConfig.Env)
	filter.Add("instance-state-name", "running")

	// TODO We should actually paginate through requests, but we are just gonna
	// rely on the API returning at least 1000 results, which is bigger than the
	// size of any instance pool we want to configure, for now.
	resp, err := h.provider.Instances(nil, filter)
	if err != nil {
		return nil, err
	}

	instances := []*ec2.Instance{}
	for _, r := range resp.Reservations {
		for _, instance := range r.Instances {
			instances = append(instances, &instance)
		}
	}

	return instances, nil
}

// poolSize returns the number of running instances currently in the instance
// pool of the given instance type.
func (h *Host) poolSize(instancetype string) (int, error) {
	instances, err := h.instancesInPool(instancetype)
	if err != nil {
		return 0, err
	}

	return len(instances), nil
}

// isPoolFull returns true when the instance pool for the given instance type is
// full.
func (h *Host) isPoolFull(instancetype string) (bool, error) {
	cfg := h.instancePoolConfig(instancetype)
	if cfg == nil {
		// A non-existent config is considered full (i.e. doesn't need any more
		// new instances).
		return true, nil
	}

	poolSize, err := h.poolSize(instancetype)
	if err != nil {
		return false, err
	}

	logrus.Infof("amazon: current %s pool size: %d, configured: %d", instancetype, poolSize, cfg.Max)

	return (poolSize >= cfg.Max), nil
}

// acquireInstance removes an instance from the pool of available instances and
// returns it. We remove the "dedicated:pool=true" tag.
func (h *Host) acquireInstance(instancetype string) (*ec2.Instance, error) {
	cfg := h.instancePoolConfig(instancetype)
	if cfg == nil {
		return nil, errors.New("no pool configured")
	}

	// Attempt to acquire a lock on the instance pool (for the required
	// instance type) so that no other hostbuilder processes can acquire /
	// release from the same pool at any time.
	addr := net.JoinHostPort(h.driverConfig.Redis.Host, h.driverConfig.Redis.Port)
	mu := distsync.NewMutex(poolMutexName(instancetype), addr)

	// The lock will be valid for this duration, during which we will:
	//   1. list the instances in the pool (performs AWS API request),
	// 	 2. pick an instance to acquire,
	//   3. edit the tags of acquired instance (performs AWS API request).
	mu.Expiry = 1 * time.Minute

	if err := mu.Lock(); err != nil {
		return nil, err
	}
	defer mu.Unlock()

	instances, err := h.instancesInPool(instancetype)
	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, errors.New("pool is empty")
	}

	// Pick a random instance.
	instance := instances[rand.Intn(len(instances))]

	h.Printf("acquired instance %s from pool for %s", instance.InstanceId, instancetype)

	// Remove tag that indicates this instance is in the pool.
	tags := []ec2.Tag{{Key: poolTag}}
	if _, err := h.provider.DeleteTags([]string{instance.InstanceId}, tags); err != nil {
		return nil, err
	}

	return instance, nil
}

// releaseInstance releases an instance to the pool of available instances. We
// indicate that an instance is in a pool by adding a "dedicated:pool=true" tag.
func (h *Host) releaseInstance() error {
	instancetype := h.instance.InstanceType

	// Attempt to acquire a lock on the instance pool.
	addr := net.JoinHostPort(h.driverConfig.Redis.Host, h.driverConfig.Redis.Port)
	mu := distsync.NewMutex(poolMutexName(instancetype), addr)
	mu.Expiry = 30 * time.Second // Not less than expected time needed to create tags.

	if err := mu.Lock(); err != nil {
		return err
	}
	defer mu.Unlock()

	tags := []ec2.Tag{
		{Key: poolTag, Value: "true"},
		{Key: "Name", Value: "userhost-pool"},
	}
	if _, err := h.provider.CreateTags([]string{h.instance.InstanceId}, tags); err != nil {
		return err
	}

	return nil
}

func poolMutexName(instancetype string) string {
	return fmt.Sprintf("pool-%s", instancetype)
}
