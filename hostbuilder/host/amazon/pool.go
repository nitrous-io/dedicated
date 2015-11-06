package amazon

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"

	"github.com/nitrous-io/dedicated/pkg/distsync"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
)

/*
 * Instance pool-related code.
 */

var poolTag = "dedicated:pool"

var (
	errPoolNotConfigured = errors.New("no pool configured")
	errPoolEmpty         = errors.New("pool is empty")
)

// returns map of region name -> (instance type -> count)
func (h *Host) ListPools() (map[string]map[string]int, error) {
	start := time.Now()
	label := h.MetricsLabel("pooled_instances")
	defer func() {
		mh.UpdateSince(label, start)
	}()

	var (
		mu      sync.Mutex
		r       = make(map[string]map[string]int)
		errmsgs []error

		wg sync.WaitGroup
	)

	for _, region := range KnownRegions {
		wg.Add(1)
		go func(region aws.Region) {
			defer wg.Done()
			var ( // goroutine locals to reduce mutex locks
				errs   []error
				counts = make(map[string]int)
			)
			conn := ec2.New(h.auth, region)

			pooled, err := h.instancesInPool(conn, "")
			if err != nil {
				errs = append(errs, err)
				return
			}
			for _, inst := range pooled {
				counts[inst.InstanceType] += 1
			}
			mu.Lock()
			errmsgs = append(errmsgs, errs...)
			r[region.Name] = counts
			mu.Unlock()

			return
		}(region)
	}
	wg.Wait()
	if len(errmsgs) > 0 {
		return nil, fmt.Errorf("pooledInstances(): %v", errmsgs)
	}
	return r, nil
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
// When instancetype is an empty value, all instance types are returned
func (h *Host) instancesInPool(provider instanceProvider, instancetype string) ([]*ec2.Instance, error) {
	filter := ec2.NewFilter()
	if instancetype != "" {
		filter.Add("instance-type", instancetype)
	}
	filter.Add(fmt.Sprintf(`tag:%s`, poolTag), "true")
	filter.Add(`tag:dedicated`, h.driverConfig.Env)
	filter.Add("instance-state-name", "running")

	// TODO We should actually paginate through requests, but we are just gonna
	// rely on the API returning at least 1000 results, which is bigger than the
	// size of any instance pool we want to configure, for now.
	resp, err := provider.Instances(nil, filter)
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
func (h *Host) poolSize(provider instanceProvider, instancetype string) (int, error) {
	instances, err := h.instancesInPool(provider, instancetype)
	if err != nil {
		return 0, err
	}

	return len(instances), nil
}

// isPoolFull returns true when the instance pool for the given instance type is
// full.
func (h *Host) isPoolFull(provider instanceProvider, instancetype string) (bool, error) {
	cfg := h.instancePoolConfig(instancetype)
	if cfg == nil {
		// A non-existent config is considered full (i.e. doesn't need any more
		// new instances).
		return true, nil
	}

	poolSize, err := h.poolSize(provider, instancetype)
	if err != nil {
		return false, err
	}

	logrus.Infof("amazon: current %s pool size: %d, configured: %d", instancetype, poolSize, cfg.Max)

	return (poolSize >= cfg.Max), nil
}

// acquireInstance removes an instance from the pool of available instances and
// returns it. We remove the "dedicated:pool=true" tag.
func (h *Host) acquireInstance(provider instanceProvider, instancetype string) (*ec2.Instance, error) {
	cfg := h.instancePoolConfig(instancetype)
	if cfg == nil {
		return nil, errPoolNotConfigured
	}

	// Attempt to acquire a lock on the instance pool (for the required
	// instance type) so that no other hostbuilder processes can acquire
	// from the same pool at any time.
	addr := net.JoinHostPort(h.driverConfig.Redis.Host, h.driverConfig.Redis.Port)
	mu := distsync.NewMutex(h.poolMutexName(instancetype), addr)

	// The lock will be valid for this duration, during which we will:
	//   1. list the instances in the pool (performs AWS API request),
	// 	 2. pick an instance to acquire,
	//   3. edit the tags of acquired instance (performs AWS API request).
	mu.Expiry = 1 * time.Minute

	if err := mu.Lock(); err != nil {
		return nil, err
	}
	defer mu.Unlock()

	instances, err := h.instancesInPool(provider, instancetype)
	if err != nil {
		return nil, err
	}

	if len(instances) == 0 {
		return nil, errPoolEmpty
	}

	// Pick the first instance (there is no point randomizing since we lock at
	// the pool level, not the instance level).
	instance := instances[0]

	h.Printf("acquired instance %s from pool for %s", instance.InstanceId, instancetype)

	// Change tag from 'true' to a timestamp to indicate time instance was acquired
	// This time is later used as the `LaunchTime` of the instance
	tags := []ec2.Tag{
		{Key: poolTag, Value: strconv.FormatInt(time.Now().Unix(), 10)},
	}
	if _, err := provider.CreateTags([]string{instance.InstanceId}, tags); err != nil {
		return nil, err
	}

	return instance, nil
}

func (h *Host) poolMutexName(instancetype string) string {
	return fmt.Sprintf("pool-%s-%s", h.Config.RegionID, instancetype)
}
