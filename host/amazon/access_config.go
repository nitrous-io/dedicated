package amazon

import (
	"fmt"

	"github.com/mitchellh/goamz/aws"
)

// AccessConfig is for common configuration related to AWS access
type AccessConfig struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	RawRegion string `json:"region"`
}

// Auth returns a valid aws.Auth object for access to AWS services, or
// an error if the authentication couldn't be resolved.
func (c *AccessConfig) Auth() (aws.Auth, error) {
	return aws.GetAuth(c.AccessKey, c.SecretKey)
}

// Region returns the aws.Region object for access to AWS services, requesting
// the region from the instance metadata if possible.
func (c *AccessConfig) Region() (aws.Region, error) {
	region, ok := aws.Regions[c.RawRegion]
	if !ok {
		return region, fmt.Errorf("Invalid region: %v", c.RawRegion)
	}
	return region, nil
}
