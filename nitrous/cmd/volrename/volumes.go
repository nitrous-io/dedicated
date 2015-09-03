package main

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/goamz/ec2"
)

func volumes() (map[string]ec2.Volume, error) {
	var (
		vols    = map[string]ec2.Volume{}
		errmsgs []string
		mutex   sync.Mutex // protects vols and errmsgs
		wg      sync.WaitGroup
	)

	for _, region := range Regions {
		wg.Add(1)
		go func(region *ec2.EC2) {
			defer wg.Done()

			regionVols := []ec2.Volume{}
			params := make(map[string]string)
			// this version is needed for the MaxResults parameter on
			// DescribeVolumes
			params["Version"] = "2015-04-15"
			params["MaxResults"] = "200"

			nextToken := "initial"
			for nextToken != "" {
				if nextToken != "" && nextToken != "initial" {
					params["NextToken"] = nextToken
				}
				vr, err := region.VolumesWithExtraParams(nil, nil, params)
				if err != nil {
					errmsg := fmt.Sprintf("failed to get volumes in region %s, err %+v", region.Name, err)
					logrus.Error(errmsg)
					mutex.Lock()
					errmsgs = append(errmsgs, errmsg)
					mutex.Unlock()
					break
				}
				regionVols = append(regionVols, vr.Volumes...)
				nextToken = vr.NextToken
			}
			mutex.Lock()
			for _, v := range regionVols {
				vols[v.VolumeId] = v
			}
			mutex.Unlock()
		}(region)
	}
	wg.Wait()

	var err error
	if len(errmsgs) > 0 {
		err = fmt.Errorf("Errors getting volumes: %s", strings.Join(errmsgs, "; "))
	}
	return vols, err
}

func setName(region, volumeID, name string) error {
	if dryrun {
		return nil
	}
	conn, ok := Regions[region]
	if !ok {
		return fmt.Errorf("No region found for %s", region)
	}
	tags := []ec2.Tag{
		{Key: "Name", Value: name},
	}

	if _, err := conn.CreateTags([]string{volumeID}, tags); err != nil {
		return fmt.Errorf("failed to tag %s with tags %#v, err: %v", volumeID, tags, err)
	}

	return nil
}

func tagValue(key string, tags []ec2.Tag) string {
	for _, tag := range tags {
		if tag.Key == key {
			return tag.Value
		}
	}
	return ""
}
