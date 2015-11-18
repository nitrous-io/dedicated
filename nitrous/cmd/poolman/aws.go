package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
)

const (
	// we reject ami versions that are older than 90 days
	versionThreshold = 3 * 30 * 24 * time.Hour
)

func awsConnect() {
	var auth aws.Auth

	auth.AccessKey = appConfig.AccessKey
	auth.SecretKey = appConfig.SecretKey

	for _, r := range appConfig.Regions {
		ec2conns[r.ID] = ec2.New(auth, aws.Regions[r.ID])
	}
}

func latestAMI(region string) (string, error) {
	amis, err := availableAMIs(region)
	if err != nil {
		return "", err
	}
	amiInfos := NewAMIInfos(amis)
	sort.Sort(amiInfos)
	latest := amiInfos[len(amiInfos)-1]
	// sanity-check version is a date within last three months
	t, err := time.Parse("200601021504", latest.Version)
	if err != nil {
		return "", err
	}
	if time.Now().Sub(t) > versionThreshold {
		return "", fmt.Errorf("Version %s is too old", latest.Version)
	}
	return latest.ID, nil
}

// store version here to avoid repeated tag lookups
type amiInfo struct {
	ID      string
	Version string
}

// sorts amis by the timestamp in 'version' tag, oldest first
type amiSorter []amiInfo

func NewAMIInfos(imgs []ec2.Image) amiSorter {
	infos := make([]amiInfo, len(imgs))
	for _, img := range imgs {
		infos = append(infos, amiInfo{
			ID:      img.Id,
			Version: getTag(img.Tags, "version"),
		})
	}
	return infos
}

func (s amiSorter) Len() int {
	return len(s)
}

func (s amiSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s amiSorter) Less(i, j int) bool {
	// version is a YYYYMMDDHHmm string
	return s[i].Version < s[j].Version
}

func availableAMIs(region string) ([]ec2.Image, error) {
	filter := ec2.NewFilter()
	filter.Add("is-public", "false")
	filter.Add("tag:hotswap", "true")
	if appConfig.Env == "production" {
		filter.Add("tag:production-ready", "true")
	}
	resp, err := ec2conns[region].Images(nil, filter)
	if err != nil {
		return nil, err
	}
	return resp.Images, nil
}

func getTag(tags []ec2.Tag, byKey string) string {
	for _, t := range tags {
		if t.Key == byKey {
			return t.Value
		}
	}
	return ""
}
