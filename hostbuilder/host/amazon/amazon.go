package amazon

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"golang.org/x/net/context"
)

const (
	DefaultPort uint16 = 65333
	sshPort     uint16 = 65100

	// represents 1gb of storage
	storageUnit = 1
	// minimum storage size in GB. This is the size of standard root disk used in AMI
	minStorageSize = 4
	// maximum storage size in GB. This is also the maximum size of a single
	// EBS volume
	// http://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_EbsBlockDevice.html
	maxStorageSize = 1024

	GeneralVolumeType  = "gp2"
	IOPSVolumeType     = "io1"
	MagneticVolumeType = "standard"
)

var (
	// Paths to files within each DigitalOcean host.
	PubKeysLocation = "/home/ubuntu/.ssh/authorized_keys"
	TLSCALocation   = "/etc/freighterd/ca.crt"
	TLSCertLocation = "/etc/freighterd/cert.crt"
	TLSKeyLocation  = "/etc/freighterd/key.crt"
	SecretLocation  = "/etc/freighterd/secret"
	ConfigLocation  = "/etc/freighterd/config.json"

	DefaultCreateTimeout = 10 * time.Minute

	ErrCreateTimeout    = errors.New("timed out creating host")
	ErrInstanceNotFound = errors.New("instance not found")

	KnownRegions = map[string]aws.Region{
		aws.APNortheast.Name:  aws.APNortheast,
		aws.APSoutheast.Name:  aws.APSoutheast,
		aws.APSoutheast2.Name: aws.APSoutheast2,
		aws.EUWest.Name:       aws.EUWest,
		aws.EUCentral.Name:    aws.EUCentral,
		aws.USEast.Name:       aws.USEast,
		aws.USWest.Name:       aws.USWest,
		aws.USWest2.Name:      aws.USWest2,
		aws.SAEast.Name:       aws.SAEast,
	}
)

// configFile is used to marshal/unmarshal /etc/freighter/config.json.
type configFile struct {
	DataPath  string       `json:"data_path"`
	TLSVerify bool         `json:"tls_verify"`
	TLSCA     string       `json:"tls_ca"`
	TLSCert   string       `json:"tls_cert"`
	TLSKey    string       `json:"tls_key"`
	Secret    string       `json:"secret"`
	NotifyURL string       `json:"notify_url"`
	Listen    []listenSpec `json:"listen"`
}

type listenSpec struct {
	Proto   string `json:"proto"`
	Address string `json:"address"`
}

type tlsConfig struct {
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type HostConfig struct {
	ID              string `json:"id"`
	HostID          int64  `json:"host_id"` // database id of the host
	Name            string `json:"name"`
	SourceAMI       string `json:"source_ami"`
	InstanceType    string `json:"instance_type"`
	RegionID        string `json:"region_id"`
	SecurityGroupID string `json:"security_group_id"`
	IPAddress       string `json:"ip_address"`

	// DisableEIP disables the allocation of Elastic IP for the current host
	// When this happens, the returned IP will be the host's public IP.
	DisableEIP bool `json:"disable_eip"`

	// VPC specific settings
	SubnetID string `json:"subnet_id"`

	// StorageSize is used to calculate total storage of the root device.
	// TotalStorage = StorageSize * storageUnit
	StorageSize int `json:"storage_size"`

	// freighter specific config
	PublicKeys        []string   `json:"public_keys"`
	TLS               *tlsConfig `json:"tls"`
	Secret            string     `json:"secret"`
	AptUsername       string     `json:"apt_username"` // host id on nitrous.io
	AptPassword       string     `json:"apt_password"` // host secret
	AptLocation       string     `json:"apt_location"` // URI of the repository
	AptDist           string     `json:"apt_dist"`     // distribution of the repo
	FreighterdVersion string     `json:"freighterd_version"`
}

func (h *HostConfig) securityGroups() []ec2.SecurityGroup {
	return []ec2.SecurityGroup{
		ec2.SecurityGroup{
			Id: h.SecurityGroupID,
		},
	}
}

type Host struct {
	AccessConfig *AccessConfig
	Config       *HostConfig
	notifyURL    string // to be set in builder's config
	uuid         string // request UUID set by user
	env          string // env tag to be set for each instance

	// ec2 properties
	auth     aws.Auth // this is used when we want to query all known regions
	provider *ec2.EC2
	instance *ec2.Instance

	// the following settings are only available during the creation phase
	keypair       *keypair      // temporary keypair of the instance
	createTimeout time.Duration // limit of instance's creation time
	conn          *sshutil.Conn // ssh connection using the temporary keypair
}

func (h *Host) Create(w io.Writer) (*host.Info, error) {
	// ctx is the Context for this method. Calling cancel closes the ctx.Done
	// channel, which will in turn signal for child requests started by this
	// method to be cancelled.
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), h.createTimeout)
	defer cancel()

	defer func() {
		if h.keypair != nil {
			fmt.Fprintf(w, "Removing key...")
			h.Logf("keyID=%s Removing key...", h.keypair.name)
			if err := h.keypair.Delete(); err != nil {
				h.progressLog(w, "Removing key...Failed!")
				h.Logf("ERROR: Failed to remove key %s", h.keypair.name)
			} else {
				h.progressLog(w, "Removing key...Done!")
			}
		}
	}()

	hctx := host.NewContext(ctx, h, w)
	steps := []host.Step{
		{
			Description: "Creating key",
			Func:        createKeypair,
		},
		{
			Description: "Checking image's availability",
			Func:        inspectSourceAMI,
		},
		{
			Description: "Creating workspace",
			Func:        createInstance,
			Timeout:     5 * time.Minute,
		},
		{
			Description: "Setting workspace's name",
			Func:        associateInstanceTags,
		},
		{
			Description: "Assigning network address",
			Func:        allocateElasticIP,
		},
		{
			Description: "Waiting for the workspace to be ready",
			Func:        waitForSSH,
			Timeout:     5 * time.Minute,
		},
		{
			Description: "Populating settings",
			Func:        writeSettings,
		},
		{
			Description: "Fetching updates",
			Func: func(ctx *host.HostContext) error {
				return host.UpdateSoftware(h, h.conn, h.Config.FreighterdVersion, ctx.Done())
			},
		},
		{
			Description: "Applying settings",
			Func: func(ctx *host.HostContext) error {
				return host.RestartServices(h, h.conn, ctx.Done())
			},
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			h.Logf("ERROR: Failed in create host step %q: %v", step.Description, err)
			h.logFailed()
			inf, _ := h.Info()
			if err == context.DeadlineExceeded {
				return inf, ErrCreateTimeout
			}
			return inf, err
		}
	}

	h.progressLog(w, "Provisioning host...Done!")
	return h.Info()
}

// Update sets properties of an existing host
func (h *Host) Update(w io.Writer) (*host.Info, error) {
	label := h.metricsLabel("update")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hctx := host.NewContext(ctx, h, w)
	steps := []host.Step{
		{
			Description: "Setting workspace's name",
			Func:        associateInstanceTags,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			h.Logf("ERROR: Failed in update host step: %v", err)
			h.logFailed()
			if err == context.DeadlineExceeded {
				return nil, ErrCreateTimeout
			}
			return nil, err
		}
	}

	return h.Info()
}

// Destroy releases the instance Elastic IP and terminate it
func (h *Host) Destroy() error {
	label := h.metricsLabel("destroy")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		// if the instance does not exist, then consider that it is sucessfully destroyed
		if err == ErrInstanceNotFound {
			return nil
		}
		return mh.Error(err, label)
	}

	// if eip is disabled for this host, we dont need to release it
	if !h.Config.DisableEIP {
		if ip := h.Config.IPAddress; ip != "" {
			// release elastic IP stored in the client. We cant use
			// h.instance.PublicIPAddress because it's not guaranteed to be
			// the correct IP when the instance is stopped
			if err := releaseAddress(h.provider, ip, isVPC(h.instance)); err != nil {
				err = fmt.Errorf("amazon: failed to release ElasticIP %s for %s, err %v",
					h.Config.IPAddress, h.instance.InstanceId, err)
				return mh.Error(err, label)
			}
		}
	}

	// terminate the instance
	if _, err := h.provider.TerminateInstances([]string{h.instance.InstanceId}); err != nil {
		err = fmt.Errorf("amazon: failed to terminate instance %s, err %v", h.instance.InstanceId, err)
		return mh.Error(err, label)
	}

	state := StateChangeConf{
		Pending: []string{"pending", "running", "shutting-down", "stopped", "stopping"},
		Refresh: InstanceStateRefreshFunc(h.provider, h.instance),
		Target:  "terminated",
	}
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		return mh.Error(err, label)
	}
	h.instance = updatedInstance.(*ec2.Instance)

	return nil
}

func (h *Host) Info() (*host.Info, error) {
	if h.instance == nil {
		return nil, errors.New("Invalid ec2 instance")
	}
	return &host.Info{
		Provider:       h.DriverName(),
		ID:             h.instance.InstanceId,
		IPAddress:      h.instance.PublicIpAddress,
		FreighterdPort: DefaultPort,
		Region:         h.provider.Region.Name,
		State:          getState(h.instance),
	}, nil
}

func (h *Host) Stop() (*host.Info, error) {
	label := h.metricsLabel("stop")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	if _, err := h.provider.StopInstances(h.instance.InstanceId); err != nil {
		return nil, mh.Error(err, label)
	}

	state := StateChangeConf{
		Pending: []string{"pending", "running", "stopping"},
		Refresh: InstanceStateRefreshFunc(h.provider, h.instance),
		Target:  "stopped",
	}
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		return nil, mh.Error(err, label)
	}
	h.instance = updatedInstance.(*ec2.Instance)

	return h.Info()
}

// Start starts the instance and allocate the desired IP address to it
func (h *Host) Start() (*host.Info, error) {
	label := h.metricsLabel("start")
	defer mh.UpdateSince(label, time.Now())

	if err := h.reloadInstance(h.Config.ID); err != nil {
		return nil, mh.Error(err, label)
	}

	if _, err := h.provider.StartInstances(h.instance.InstanceId); err != nil {
		return nil, mh.Error(err, label)
	}

	state := StateChangeConf{
		Pending: []string{"pending", "stopped"},
		Refresh: InstanceStateRefreshFunc(h.provider, h.instance),
		Target:  "running",
	}
	updatedInstance, err := WaitForState(&state)
	if err != nil {
		return nil, mh.Error(err, label)
	}
	h.instance = updatedInstance.(*ec2.Instance)

	// if this is a VPC instance, we dont have to reassociate ElasticIP because
	// it will be persisted after stop by default.
	// http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/elastic-ip-addresses-eip.html#VPC_EIP_EC2_Differences
	if !isVPC(h.instance) && !h.Config.DisableEIP {
		if _, err := h.provider.AssociateAddress(&ec2.AssociateAddress{
			InstanceId:         h.instance.InstanceId,
			PublicIp:           h.Config.IPAddress,
			AllowReassociation: false,
		}); err != nil {
			return nil, mh.Error(err, label)
		}

		// reload the instance to get a new public IP address
		if err := h.reloadInstance(h.instance.InstanceId); err != nil {
			return nil, mh.Error(err, label)
		}
	}

	return h.Info()
}

// Restart performs a hard-reset of the instance. This should not change the
// public IP address of the instance. This does not use RebootAPI, but instead
// stop/starting the instance to make sure that it has a clean state.
func (h *Host) Restart() (*host.Info, error) {
	label := h.metricsLabel("restart")
	defer mh.UpdateSince(label, time.Now())
	if _, err := h.Stop(); err != nil {
		return nil, err
	}
	return h.Start()
}

// List returns all instances in the current region. To perform list of
// multiple regions, use one List() request for each region.
func (h *Host) List() ([]*host.Info, error) {
	label := h.metricsLabel("list")
	defer mh.UpdateSince(label, time.Now())

	var (
		mu    = &sync.Mutex{}
		infos = []*host.Info{}
		wg    sync.WaitGroup
	)

	// create a filter for the current environment
	filter := ec2.NewFilter()
	filter.Add("tag:dedicated", h.env)

	for _, region := range KnownRegions {
		conn := ec2.New(h.auth, region)
		wg.Add(1)
		go func(conn *ec2.EC2, region aws.Region) {
			defer wg.Done()
			instancesResp, err := conn.Instances(nil, filter)
			if err != nil {
				h.Logf("failed to get instances in region %s, err %v", region.Name, err)
				return
			}
			mu.Lock()
			for _, reservation := range instancesResp.Reservations {
				for _, instance := range reservation.Instances {
					info := &host.Info{
						Provider:       h.DriverName(),
						ID:             instance.InstanceId,
						IPAddress:      instance.PublicIpAddress,
						FreighterdPort: DefaultPort,
						State:          getState(&instance),
						Region:         region.Name,
					}

					// Get EC2 instance name
					for _, tag := range instance.Tags {
						if tag.Key == "Name" {
							info.Name = tag.Value
							break
						}
					}

					infos = append(infos, info)
				}
			}
			mu.Unlock()
		}(conn, region)
	}

	wg.Wait()
	return infos, nil
}

func (h *Host) DriverName() string {
	return "amazon"
}

// Logf logs the data together with the host uuid and droplet id if they are
// avalaible
func (h *Host) Logf(format string, v ...interface{}) {
	parts := []string{}
	if h.uuid != "" {
		parts = append(parts, fmt.Sprintf("uuid=%s", h.uuid))
	}
	if h.instance != nil {
		parts = append(parts, fmt.Sprintf("instance=%s", h.instance.InstanceId))
	}
	log.Printf(fmt.Sprintf("amazon: %s %s", strings.Join(parts, " "), format), v...)
}

// MetricsLabel returns the label with the current host's region if possible
func (h *Host) MetricsLabel(prefix string) string {
	// region should be set for every requests because ec2 API requires it to
	// find the instance id properly
	if h.Config.RegionID != "" {
		return fmt.Sprintf("%s.amazon.%s", prefix, h.Config.RegionID)
	}
	return fmt.Sprintf("%s.amazon", prefix)
}

// refreshInstance reload the current instance associated with this Host to the
// instance specified by `id` params. It can be used to refresh the current
// instance, or load a new one
func (h *Host) reloadInstance(id string) error {
	instancesResp, err := h.provider.Instances([]string{id}, nil)
	if err != nil {
		return err
	}
	if len(instancesResp.Reservations) == 0 || len(instancesResp.Reservations[0].Instances) == 0 {
		return ErrInstanceNotFound
	}
	h.instance = &instancesResp.Reservations[0].Instances[0]
	return nil
}

func (h *Host) configFile() *configFile {
	return &configFile{
		DataPath:  "/var/lib/freighter",
		TLSVerify: true,
		TLSCA:     TLSCALocation,
		TLSCert:   TLSCertLocation,
		TLSKey:    TLSKeyLocation,
		Secret:    SecretLocation,
		NotifyURL: h.generateNotifyURL(),
		Listen: []listenSpec{
			{Proto: "unix", Address: "/var/run/freighterd.sock"},
			{Proto: "tcp", Address: fmt.Sprintf("0.0.0.0:%d", DefaultPort)},
		},
	}
}

func (h *Host) generateNotifyURL() string {
	return strings.Replace(h.notifyURL, "{{slug}}", strconv.FormatInt(h.Config.HostID, 10), -1)
}

// progressLog writes the message to the given writer and logs the message,
// prefixing host information.  It is used for user-facing messages.  Use
// `h.Logf` for internal logging.
func (h *Host) progressLog(w io.Writer, format string, v ...interface{}) {
	fmt.Fprintf(w, format, v...)
	h.Logf(format, v...)
}

func (h *Host) logFailed() {
	if h.instance != nil {
		if h.keypair != nil {
			h.Logf("keyID=%s Leaving instance for inspection: %s", h.keypair.name, h.instance.InstanceId)
		} else {
			h.Logf("keyID=unknown Leaving instance for inspection: %s", h.instance.InstanceId)
		}
	}
}

// mlabel returns provider.region which is probably helpful if we ever want to
// differentiate between regions.
//
// The result will have a form of prefix.amazon<.region>
func (h *Host) metricsLabel(prefix string) string {
	// region should be set for every requests because ec2 API requires it to
	// find the instance id properly
	if h.Config.RegionID != "" {
		return fmt.Sprintf("%s.amazon.%s", prefix, h.Config.RegionID)
	}
	return fmt.Sprintf("%s.amazon", prefix)
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		log.Printf("uuid=%s amazon: invalid config %s", defaultConfig["uuid"], config)
		return nil, err
	}
	accessConfig := &AccessConfig{
		AccessKey: defaultConfig["access_key"],
		SecretKey: defaultConfig["secret_key"],
		RawRegion: hostConfig.RegionID,
	}
	auth, err := accessConfig.Auth()
	if err != nil {
		log.Printf("uuid=%s amazon: auth failed %v", defaultConfig["uuid"], err)
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
		auth:          auth,
		createTimeout: createTimeout,
		uuid:          defaultConfig["uuid"], // this is injected by the server per request
		notifyURL:     defaultConfig["notify_url"],
		env:           defaultConfig["env"],
	}

	// workaround for actions that requires multiple regions i.e. Host#List
	// This is set by the caller and should not be serialized as a
	// configuration value
	if _, ok := defaultConfig["ignore_region"]; !ok {
		region, err := accessConfig.Region()
		if err != nil {
			log.Printf("uuid=%s amazon: invalid region %v", defaultConfig["uuid"], err)
			return nil, err
		}
		host.provider = ec2.New(auth, region)
	}
	return host, nil
}

func init() {
	host.Hosts["amazon"] = NewHost
}

// createKeypair create a temporary keypair using Amazon API, store the
// temporary private key so we can connect to the host during the creation
// phase. The keypair should be deleted after provisioning finishes.
func createKeypair(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("create_key")
	defer mh.UpdateSince(label, time.Now())

	// generate key pair
	k, err := generateTempKeypair(h.provider)
	if err != nil {
		return mh.Error(err, label)
	}
	h.keypair = k
	return nil
}

// inspectSourceAMI checks that the requested AMI exists and can be used with
// the current settings.
func inspectSourceAMI(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("inspect_source_ami")
	defer mh.UpdateSince(label, time.Now())

	h.Logf("inspecting source ami %q", h.Config.SourceAMI)
	imageResp, err := h.provider.Images([]string{h.Config.SourceAMI}, ec2.NewFilter())
	if err != nil {
		return mh.Error(fmt.Errorf("amazon: failed to list images %s", err), label)
	}

	if len(imageResp.Images) == 0 {
		return mh.Error(fmt.Errorf("amazon: image %q not found", h.Config.SourceAMI), label)
	}

	h.Logf("inspecting source ami %q... Done", h.Config.SourceAMI)
	return nil
}

// createInstance runs the instance with specified config and a temporary
// keypair and wait for it to be ready.
func createInstance(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("create_host")
	defer func(start time.Time) {
		// we are interested in the final context of `h` so label is generated
		// inside defer block
		mh.UpdateSince(label, start)
	}(time.Now())

	storageSize := int64(h.Config.StorageSize * storageUnit)
	if storageSize > maxStorageSize {
		storageSize = maxStorageSize
	} else if storageSize < minStorageSize {
		storageSize = minStorageSize
	}

	h.Logf("amazon: creating instance in region %s, type: %s, size: %d, slug: %s",
		h.Config.RegionID, h.Config.InstanceType, storageSize, h.Config.Name)

	opts := &ec2.RunInstances{
		KeyName:        h.keypair.name,
		ImageId:        h.Config.SourceAMI,
		InstanceType:   h.Config.InstanceType,
		SecurityGroups: h.Config.securityGroups(),
		MinCount:       0,
		MaxCount:       0,
		BlockDevices: []ec2.BlockDeviceMapping{
			{
				DeviceName:          "/dev/sda1",
				VolumeType:          GeneralVolumeType,
				VolumeSize:          storageSize,
				DeleteOnTermination: true,
			},
		},
		AssociatePublicIpAddress: true,
	}
	// assigned subnet ID for VPC instance
	if h.Config.SubnetID != "" {
		opts.SubnetId = h.Config.SubnetID
	}
	resp, err := h.provider.RunInstances(opts)
	if err != nil || len(resp.Instances) == 0 {
		return mh.Error(fmt.Errorf("amazon: failed to create new instance %v", err), label)
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
		return mh.Error(fmt.Errorf("amazon: failed to change state %v", err), label)
	}
	h.instance = updatedInstance.(*ec2.Instance)

	if h.instance.PublicIpAddress == "" {
		return fmt.Errorf("failed to allocate public IP for instance %s", h.instance.InstanceId)
	}

	return nil
}

// allocateElasticIP creates a new ElasticIP and associate it with the
// instance. This is done so the public IP of the instance will not be changed
// when it's stopped/restarted
func allocateElasticIP(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	// if eip is disabled on this host, we dont want to allocate it
	if h.Config.DisableEIP {
		return nil
	}
	label := h.metricsLabel("allocate_ip")
	defer mh.UpdateSince(label, time.Now())

	_, err := assignAddress(h.provider, h.instance.InstanceId, isVPC(h.instance))
	if err != nil {
		return mh.Error(err, label)
	}

	// reload the instance to get a new public IP address
	if err := h.reloadInstance(h.instance.InstanceId); err != nil {
		return mh.Error(err, label)
	}

	return nil
}

// associateInstanceTags sets the instance's tags which include Name and
// dedicated environment.
// If this fails, we still consider the operation to be successful
func associateInstanceTags(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("set_name")
	defer mh.UpdateSince(label, time.Now())

	tags := []ec2.Tag{{"Name", h.Config.Name}}
	if h.env != "" {
		tags = append(tags, ec2.Tag{"dedicated", h.env})
	}
	_, err := h.provider.CreateTags([]string{h.instance.InstanceId}, tags)
	if err != nil {
		h.Logf("Failed to set name for instance %s", h.instance.InstanceId)
	}
	return nil
}

// waitForSSH creates a SSH connection to the host using the temporary keypair
// so we can perform setup actions on it
func waitForSSH(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("ssh_wait")
	defer mh.UpdateSince(label, time.Now())

	h.Logf("Waiting for SSH to be ready")
	address := fmt.Sprintf("%s:%d", h.instance.PublicIpAddress, sshPort)
	info := map[string]interface{}{
		"uuid": h.uuid,
	}
	connInterval := 5 * time.Second
	sshConn, err := sshutil.WaitForSSH(address, "ubuntu", h.keypair.privateKey,
		connInterval, ctx.Done(), info)
	if err != nil {
		return mh.Error(err, label)
	}
	h.Logf("SSH ready for instance %s", h.instance.InstanceId)
	h.conn = sshConn
	return nil
}

// writeSettings uploads necessary file system contents to the host, which
// include package configs, apt details, etc...
func writeSettings(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("write_settings")
	defer mh.UpdateSince(label, time.Now())
	h.Logf("Writing settings")

	pubKeysBuf := bytes.NewBuffer(nil)
	for _, k := range h.Config.PublicKeys {
		fmt.Fprintln(pubKeysBuf, k)
	}

	aptSourcesBuf := bytes.NewBuffer(nil)
	if err := host.SourceTemplate.Execute(aptSourcesBuf, h.Config); err != nil {
		return mh.Error(err, label)
	}

	cfg := h.configFile()
	cfgb, err := json.Marshal(cfg)
	if err != nil {
		return mh.Error(fmt.Errorf("unable to marshal config JSON, err: %v", err), label)
	}

	if err := h.conn.Execute("sudo mkdir -p /etc/freighterd"); err != nil {
		return mh.Error(err, label)
	}

	if err := uploadFile(h, map[string]io.Reader{
		PubKeysLocation:     pubKeysBuf,
		host.SourceLocation: aptSourcesBuf,
		TLSCALocation:       strings.NewReader(h.Config.TLS.CA),
		TLSCertLocation:     strings.NewReader(h.Config.TLS.Cert),
		TLSKeyLocation:      strings.NewReader(h.Config.TLS.Key),
		SecretLocation:      strings.NewReader(h.Config.Secret),
		ConfigLocation:      bytes.NewBuffer(cfgb),
	}); err != nil {
		return err
	}

	h.Logf("Wrote settings")
	return nil
}

// uploadFile uploads a set of files to the remote host using the temporary SSH
// connection. This is only available during creation phase. The host
// filesystem should be considered immutable otherwise.
func uploadFile(h *Host, files map[string]io.Reader) error {
	label := h.metricsLabel("update_software")
	defer mh.UpdateSince(label, time.Now())

	flist := []string{}
	for dest, val := range files {
		tmpDest := filepath.Join("/tmp", hex.EncodeToString([]byte(dest)))
		if err := h.conn.Upload(tmpDest, val); err != nil {
			return mh.Error(err, label)
		}
		if err := h.conn.Execute(fmt.Sprintf("sudo mv %s %s", tmpDest, fmt.Sprintf("%q", dest))); err != nil {
			return mh.Error(err, label)
		}
		flist = append(flist, dest)
	}
	return nil
}

func getState(instance *ec2.Instance) state.State {
	switch instance.State.Name {
	case "pending":
		return state.Starting
	case "running":
		return state.Running
	case "stopping":
		return state.Stopping
	case "stopped":
		return state.Stopped
	case "terminated":
		return state.Terminated
	}
	return state.Unknown
}

// isVPC checks if an instance is a EC2-VPC instance
func isVPC(i *ec2.Instance) bool {
	return i != nil && i.SubnetId != ""
}
