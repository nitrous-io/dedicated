package amazon

import (
	"time"

	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
)

var defaultCreateTimeout = 10 * time.Minute

// driverConfig stores driver configuration, such as Amazon EC2 access/secret
// keys, timeouts.
type driverConfig struct {
	AccessKey     string `json:"access_key"`
	SecretKey     string `json:"secret_key"`
	HostVersion   string `json:"host_version"` // Created instances will be tagged with "dedicated:version=#{HostVersion}"
	Env           string `json:"env"`          // Created instances will be tagged with "dedicated=#{Env}".
	CreateTimeout string `json:"create_timeout"`
	NotifyURL     string `json:"notify_url"`

	Regions []regionConfig `json:"regions"`

	Redis redisConfig `json:"redis"`

	// TODO Super hacky - refactor this when we refactor hostbuilder drivers.
	IgnoreRegion string `json:"ignore_region"`

	// cachedCreateTimeout is used to cache a parsed CreateTimeout.
	cachedCreateTimeout time.Duration
}

type regionConfig struct {
	ID           string               `json:"id"`
	Keypair      keypairConfig        `json:"keypair"`
	InstancePool []instancePoolConfig `json:"instance_pool"`
}

type keypairConfig struct {
	Name           string `json:"name"`
	PrivateKeyPath string `json:"private_key_path"`
}

type instancePoolConfig struct {
	Type string `json:"type"`
	Max  int    `json:"max"`
}

type redisConfig struct {
	Host string `json:"host"`
	Port string `json:"port"`
}

func (c *driverConfig) createTimeout() time.Duration {
	if c.cachedCreateTimeout != 0 {
		return c.cachedCreateTimeout
	}

	t := defaultCreateTimeout
	d, err := time.ParseDuration(c.CreateTimeout)
	if err == nil {
		t = d
	}
	c.cachedCreateTimeout = t

	return t
}

type hostConfig struct {
	ID       string `json:"id"`
	HostID   int64  `json:"host_id"` // database id of the host
	VolumeID string `json:"volume_id"`

	Name            string `json:"name"`
	SourceAMI       string `json:"source_ami"`
	InstanceType    string `json:"instance_type"`
	RegionID        string `json:"region_id"`
	SecurityGroupID string `json:"security_group_id"`
	IPAddress       string `json:"ip_address"`
	Tags            string `json:"tags"`

	// DisableEIP disables the allocation of Elastic IP for the current host
	// When this happens, the returned IP will be the host's public IP.
	DisableEIP bool `json:"disable_eip"`

	// VPC specific settings
	SubnetID string `json:"subnet_id"`

	// StorageSize is used to calculate total storage of the root device.
	// TotalStorage = StorageSize * storageUnit
	StorageSize int `json:"storage_size"`

	// freighter specific config
	PublicKeys  []string        `json:"public_keys"`
	TLS         *host.TLSConfig `json:"tls"`
	Secret      string          `json:"secret"`
	AptUsername string          `json:"apt_username"` // host id on nitrous.io
	AptPassword string          `json:"apt_password"` // host secret
	AptLocation string          `json:"apt_location"` // URI of the repository
	AptDist     string          `json:"apt_dist"`     // distribution of the repo
}

func (c *hostConfig) securityGroups() []ec2.SecurityGroup {
	return []ec2.SecurityGroup{
		{Id: c.SecurityGroupID},
	}
}
