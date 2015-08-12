package amazon

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
)

// List returns all instances in all known regions, except instances that are
// in an instance pool.
func (h *Host) List() ([]*host.Info, error) {
	label := h.MetricsLabel("list")
	defer mh.UpdateSince(label, time.Now())

	var (
		infos   = []*host.Info{}
		errmsgs []string
		mutex   = &sync.Mutex{} // protects infos and errmsgs
		wg      sync.WaitGroup
	)

	for _, region := range KnownRegions {
		wg.Add(1)
		go func(region aws.Region) {
			defer wg.Done()

			// per-region librato metrics
			// h.MetricsLabel cannot be used since it assumes region is populated
			// and List() call is region-agnostic
			label := fmt.Sprintf("%s.amazon.%s", "list", region.Name)
			defer mh.UpdateSince(label, time.Now())

			regionInfos := []*host.Info{}
			conn := ec2.New(h.auth, region)
			params := make(map[string]string)
			params["MaxResults"] = "200"

			firstTime := true
			nextToken := ""

			for firstTime || nextToken != "" {
				if nextToken != "" {
					params["NextToken"] = nextToken
				}

				instancesResp, err := conn.InstancesWithExtraParams(nil, nil, params)
				if err != nil {
					errmsg := fmt.Sprintf("failed to get instances in region %s, err %+v", region.Name, err)
					h.Printf(errmsg)
					mutex.Lock()
					errmsgs = append(errmsgs, errmsg)
					mutex.Unlock()
					break
				}

				nextToken = instancesResp.NextToken
				firstTime = false

				for _, reservation := range instancesResp.Reservations {
					for _, instance := range reservation.Instances {
						// We only care about instances that are tagged with
						// "dedicated=<driverConfig.Env>"
						env := tagValue("dedicated", instance.Tags)
						if env != h.driverConfig.Env {
							continue
						}

						// Don't return instances that are in an instance pool.
						inPool := tagValue(poolTag, instance.Tags)
						if inPool == "true" {
							continue
						}

						info := &host.Info{
							Name:           tagValue("Name", instance.Tags),
							Provider:       h.DriverName(),
							ID:             instance.InstanceId,
							IPAddress:      instance.PublicIpAddress,
							FreighterdPort: host.DefaultPort,
							State:          getState(&instance),
							Region:         region.Name,
							LaunchTime:     &instance.LaunchTime,
						}

						regionInfos = append(regionInfos, info)
					}
				}
			}
			mutex.Lock()
			infos = append(infos, regionInfos...)
			mutex.Unlock()
		}(region)
	}
	wg.Wait()

	var err error
	if len(errmsgs) > 0 {
		err = fmt.Errorf("Errors getting instances: %s", strings.Join(errmsgs, "; "))
	}
	return infos, err
}

func tagValue(key string, tags []ec2.Tag) string {
	for _, tag := range tags {
		if tag.Key == key {
			return tag.Value
		}
	}
	return ""
}
