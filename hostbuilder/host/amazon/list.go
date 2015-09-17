package amazon

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
)

// List returns all instances and volumes in all known regions, except instances
// that are in an instance pool.
func (h *Host) List() ([]*host.Info, error) {
	label := h.MetricsLabel("list")
	defer mh.UpdateSince(label, time.Now())

	var (
		infos  []*host.Info
		wg     sync.WaitGroup
		errors []error
		mu     sync.Mutex // protects infos and errors
	)

	wg.Add(1)
	go func() {
		defer wg.Done()

		allInstances, err := h.listInstances()
		if err != nil {
			mu.Lock()
			errors = append(errors, err)
			mu.Unlock()
			return
		}

		instanceInfos := []*host.Info{}
		for region, instances := range allInstances {
			for _, instance := range instances {
				istate := getState(&instance)
				if istate == state.Starting {
					// these instances are pending on ec2 (see
					// https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-instance-lifecycle.html)
					// We do not include these in the list as:
					// - they are not billed for
					// - they are not in a usable state
					// - they cause false alarms in healthcheck due to lack of
					// launch time for grace period allowances
					continue
				}

				// acquired pool instances have the time of acquisition in the
				// poolTag.  We use this as a proxy for launch time.
				ltime := &instance.LaunchTime
				poolValue := tagValue(poolTag, instance.Tags)
				// empty string will return err in Atoi; takes care of case
				// where instance is not in pool
				timeInt, err := strconv.Atoi(poolValue)
				if err == nil {
					// sanity check time value; after 2015, before 'now'
					atime := time.Unix(int64(timeInt), 0)
					threshold := time.Unix(1420070400, 0) // Jan 1 2015 00:00:00 GMT
					if atime.Before(time.Now()) && atime.After(threshold) {
						ltime = &atime
					}
				}
				info := &host.Info{
					Name:           tagValue("Name", instance.Tags),
					Provider:       h.DriverName(),
					ID:             instance.InstanceId,
					IPAddress:      instance.PublicIpAddress,
					FreighterdPort: host.DefaultPort,
					State:          istate,
					Region:         region.Name,
					LaunchTime:     ltime,
				}

				userDevice := blockDevice(&instance, userDeviceName)
				if userDevice != nil {
					info.VolumeID = userDevice.VolumeId
				}

				instanceInfos = append(instanceInfos, info)
			}
		}

		mu.Lock()
		infos = append(infos, instanceInfos...)
		mu.Unlock()
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		allVols, err := h.listVolumes()
		if err != nil {
			mu.Lock()
			errors = append(errors, err)
			mu.Unlock()
			return
		}

		volInfos := []*host.Info{}
		for region, vols := range allVols {
			for _, vol := range vols {
				info := &host.Info{
					VolumeID: vol.VolumeId,
					Name:     tagValue("Name", vol.Tags),
					Provider: h.DriverName(),
					Region:   region.Name,
				}

				if len(vol.Attachments) > 0 {
					info.ID = vol.Attachments[0].InstanceId
				}

				volInfos = append(volInfos, info)
			}
		}

		mu.Lock()
		infos = append(infos, volInfos...)
		mu.Unlock()
	}()
	wg.Wait()

	if len(errors) > 0 {
		return nil, fmt.Errorf("List(): %v", errors)
	}
	return infos, nil
}

func (h *Host) listInstances() (map[aws.Region][]ec2.Instance, error) {
	start := time.Now()
	label := h.MetricsLabel("list_instances")
	defer func() {
		mh.UpdateSince(label, start)
		h.Printf("listing instances from EC2 took %v", time.Now().Sub(start))
	}()

	var (
		allInstances = make(map[aws.Region][]ec2.Instance)
		errmsgs      []string
		mu           sync.Mutex // protects allInstances and errmsgs
		wg           sync.WaitGroup
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

			instances := []ec2.Instance{}

			conn := ec2.New(h.auth, region)
			params := map[string]string{"MaxResults": "200"}

			nextToken := "initial"
			for nextToken != "" {
				if nextToken != "" && nextToken != "initial" {
					params["NextToken"] = nextToken
				}

				instancesResp, err := conn.InstancesWithExtraParams(nil, nil, params)
				if err != nil {
					errmsg := fmt.Sprintf("failed to get instances for region %s, err: %v", region.Name, err)
					h.Printf(errmsg)
					mu.Lock()
					errmsgs = append(errmsgs, errmsg)
					mu.Unlock()
					break
				}

				nextToken = instancesResp.NextToken

				for _, reservation := range instancesResp.Reservations {
					for _, instance := range reservation.Instances {
						// We only care about instances that are tagged with
						// "dedicated=<driverConfig.Env>"
						env := tagValue("dedicated", instance.Tags)
						if env != h.driverConfig.Env {
							continue
						}

						// Ignore instances that are in an instance pool.
						inPool := tagValue(poolTag, instance.Tags)
						if inPool == "true" {
							continue
						}

						instances = append(instances, instance)
					}
				}
			}

			mu.Lock()
			allInstances[region] = instances
			mu.Unlock()
		}(region)
	}
	wg.Wait()

	if len(errmsgs) > 0 {
		return nil, fmt.Errorf("Errors getting instances: %s", strings.Join(errmsgs, "; "))
	}

	return allInstances, nil
}

func (h *Host) listVolumes() (map[aws.Region][]ec2.Volume, error) {
	start := time.Now()
	label := h.MetricsLabel("list_volumes")
	defer func() {
		mh.UpdateSince(label, start)
		h.Printf("listing volumes from EC2 took %v", time.Now().Sub(start))
	}()

	var (
		allVols = make(map[aws.Region][]ec2.Volume)
		errmsgs []string
		mu      sync.Mutex // protects allVols and errmsgs
		wg      sync.WaitGroup
	)

	for _, region := range KnownRegions {
		wg.Add(1)

		go func(region aws.Region) {
			defer wg.Done()

			vols := []ec2.Volume{}

			conn := ec2.New(h.auth, region)
			filter := ec2.NewFilter()
			filter.Add(`tag:dedicated`, h.driverConfig.Env)
			// Specify the version for the MaxResults parameter on DescribeVolumes.
			params := map[string]string{"Version": "2015-04-15", "MaxResults": "200"}

			nextToken := "initial"
			for nextToken != "" {
				if nextToken != "" && nextToken != "initial" {
					params["NextToken"] = nextToken
				}

				vr, err := conn.VolumesWithExtraParams(nil, filter, params)
				if err != nil {
					errmsg := fmt.Sprintf("failed to get volumes for region %s, err: %v", region.Name, err)
					h.Printf(errmsg)
					mu.Lock()
					errmsgs = append(errmsgs, errmsg)
					mu.Unlock()
					break
				}

				nextToken = vr.NextToken

				for _, vol := range vr.Volumes {
					name := tagValue("Name", vol.Tags)
					if !strings.HasPrefix(name, "userdata") {
						// Ignore volumes that are not user volumes.
						continue
					}

					vols = append(vols, vol)
				}
			}

			mu.Lock()
			allVols[region] = vols
			mu.Unlock()
		}(region)
	}
	wg.Wait()

	if len(errmsgs) > 0 {
		return nil, fmt.Errorf("Errors getting volumes: %s", strings.Join(errmsgs, "; "))
	}

	return allVols, nil
}

func tagValue(key string, tags []ec2.Tag) string {
	for _, tag := range tags {
		if tag.Key == key {
			return tag.Value
		}
	}
	return ""
}
