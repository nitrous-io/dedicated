package amazon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"golang.org/x/net/context"
)

var (
	DefaultCreateTimeout = 10 * time.Minute
	ErrCreateTimeout     = errors.New("timed out creating host")
)

type tlsConfig struct {
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type HostConfig struct {
	ID               string            `json:"id"`
	SourceAMI        string            `json:"source_ami"`
	AvailabilityZone string            `json:"availability_zone"`
	InstanceType     string            `json:"instance_type"`
	RawRegion        string            `json:"region"`
	RawSecurityGroup map[string]string `json:"security_group"`

	// freighter specific config
	PublicKeys        []string   `json:"public_keys"`
	TLS               *tlsConfig `json:"tls"`
	Secret            string     `json:"secret"`
	AptUsername       string     `json:"apt_username"` // username on nitrous.io
	AptPassword       string     `json:"apt_password"` // nitrous.io access token
	AptLocation       string     `json:"apt_location"` // URI of the repository
	AptDist           string     `json:"apt_dist"`     // distribution of the repo
	FreighterdVersion string     `json:"freighterd_version"`
}

func (h *HostConfig) securityGroups() []ec2.SecurityGroup {
	return []ec2.SecurityGroup{
		ec2.SecurityGroup{
			Id:   h.RawSecurityGroup["id"],
			Name: h.RawSecurityGroup["name"],
		},
	}
}

type Host struct {
	AccessConfig *AccessConfig
	Config       *HostConfig
	provider     *ec2.EC2

	instance *ec2.Instance

	// the following settings are only available during the creation phase
	keypair       *keypair      // temporary keypair of the instance
	createTimeout time.Duration // limit of instance's creation time
}

func (h *Host) Create(w io.Writer) (*host.Info, error) {
	w = io.MultiWriter(os.Stdout, w)

	// ctx is the Context for this method. Calling cancel closes the ctx.Done
	// channel, which will in turn signal for child requests started by this
	// method to be cancelled.
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), h.createTimeout)
	defer cancel()

	// generate key pair
	k, err := generateTempKeypair(h.provider)
	if err != nil {
		return nil, err
	}
	defer k.Delete()
	h.keypair = k

	hctx := host.NewContext(ctx, h, w)
	steps := []host.Step{
		{
			Description: "Checking image's availability",
			Func:        inspectSourceAMI,
		},
		{
			Description: "Creating workspace",
			Func:        createInstance,
			Timeout:     5 * time.Minute,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			if err == context.DeadlineExceeded {
				return nil, ErrCreateTimeout
			}
			return nil, err
		}
	}

	return h.Info()
}

func (h *Host) Update(w io.Writer) (*host.Info, error) {
	return nil, errors.New("amazon: Update() not implemented because we don't care")
}

func (h *Host) Destroy() error {
	var instancesResp *ec2.InstancesResp
	var err error
	if instancesResp, err = h.provider.Instances([]string{h.Config.ID}, nil); err != nil {
		return err
	}
	if _, err := h.provider.TerminateInstances([]string{h.Config.ID}); err != nil {
		return err
	}
	instance := instancesResp.Reservations[0].Instances[0]
	state := StateChangeConf{
		Pending: []string{"pending", "running", "shutting-down", "stopped", "stopping"},
		Refresh: InstanceStateRefreshFunc(h.provider, &instance),
		Target:  "terminated",
	}
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		return err
	}
	h.instance = updatedInstance.(*ec2.Instance)
	return nil
}

func (h *Host) Info() (*host.Info, error) {
	if h.instance == nil {
		return nil, errors.New("Invalid ec2 instance")
	}
	info := &host.Info{
		Provider:  h.DriverName(),
		ID:        h.instance.InstanceId,
		IPAddress: h.instance.PublicIpAddress,
	}
	return info, nil
}

func (h *Host) Stop() (*host.Info, error) {
	panic("amazon provider does not support stop yet")
}

func (h *Host) Start() (*host.Info, error) {
	panic("amazon provider does not support start yet")
}

func (h *Host) Restart() (*host.Info, error) {
	panic("amazon provider does not support Restart yet")
}

func (h *Host) List() ([]*host.Info, error) {
	panic("amazon provider does not support List yet")
}

func (h *Host) DriverName() string {
	return "amazon"
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		log.Printf("amazon: invalid config %s", config)
		return nil, err
	}
	accessConfig := &AccessConfig{
		AccessKey: defaultConfig["access_key"],
		SecretKey: defaultConfig["secret_key"],
		RawRegion: hostConfig.RawRegion,
	}
	auth, err := accessConfig.Auth()
	if err != nil {
		log.Printf("amazon: auth failed: %v", err)
		return nil, err
	}
	region, err := accessConfig.Region()
	if err != nil {
		log.Printf("amazon: invalid region: %v", err)
		return nil, err
	}
	// get create timeout
	createTimeout := DefaultCreateTimeout
	d, err := time.ParseDuration(defaultConfig["create_timeout"])
	if err == nil {
		createTimeout = d
	}

	host := &Host{
		AccessConfig:  accessConfig,
		Config:        hostConfig,
		provider:      ec2.New(auth, region),
		createTimeout: createTimeout,
	}
	return host, nil
}

func init() {
	host.Hosts["amazon"] = NewHost
}

// inspectSourceAMI checks that the requested AMI exists and can be used with
// the current settings.
func inspectSourceAMI(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)

	imageResp, err := h.provider.Images([]string{h.Config.SourceAMI}, ec2.NewFilter())
	if err != nil {
		return fmt.Errorf("amazon: failed to list images %s", err)
	}

	if len(imageResp.Images) == 0 {
		return fmt.Errorf("amazon: image %q not found", h.Config.SourceAMI)
	}

	return nil
}

// createInstance runs the instance with specified config and a temporary
// keypair and wait for it to be ready.
func createInstance(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)

	opts := &ec2.RunInstances{
		KeyName:        h.keypair.name,
		ImageId:        h.Config.SourceAMI,
		InstanceType:   h.Config.InstanceType,
		SecurityGroups: h.Config.securityGroups(),
		AvailZone:      h.Config.AvailabilityZone,
		MinCount:       0,
		MaxCount:       0,
	}
	resp, err := h.provider.RunInstances(opts)
	if err != nil || len(resp.Instances) == 0 {
		return fmt.Errorf("amazon: failed to create new instance %v", err)
	}
	h.instance = &resp.Instances[0]

	// wait for the instance to be ready
	state := &StateChangeConf{
		Pending: []string{"pending"},
		Target:  "running",
		Refresh: InstanceStateRefreshFunc(h.provider, h.instance),
	}
	updatedInstance, err := WaitForState(state)
	if err != nil {
		return fmt.Errorf("amazon: failed to change state %v", err)
	}
	h.instance = updatedInstance.(*ec2.Instance)

	return nil
}
