package amazon

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

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
)

var (
	// Paths to files within each DigitalOcean host.
	PubKeysLocation = "/root/.ssh/authorized_keys"
	TLSCALocation   = "/etc/freighterd/ca.crt"
	TLSCertLocation = "/etc/freighterd/cert.crt"
	TLSKeyLocation  = "/etc/freighterd/key.crt"
	SecretLocation  = "/etc/freighterd/secret"
	ConfigLocation  = "/etc/freighterd/config.json"

	DefaultCreateTimeout = 10 * time.Minute

	ErrCreateTimeout    = errors.New("timed out creating host")
	ErrInstanceNotFound = errors.New("instance not found")
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
	ID               string            `json:"id"`
	Name             string            `json:"name"`
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
	notifyURL    string // to be set in builder's config
	uuid         string // request UUID set by user

	// ec2 properties
	provider *ec2.EC2
	instance *ec2.Instance

	// the following settings are only available during the creation phase
	keypair       *keypair      // temporary keypair of the instance
	createTimeout time.Duration // limit of instance's creation time
	conn          *sshutil.Conn // ssh connection using the temporary keypair
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

	defer func() {
		if h.keypair != nil {
			fmt.Fprintf(w, "Removing key...")
			hlog(h, "keyID=%s Removing key...", h.keypair.name)
			if err := h.keypair.Delete(); err != nil {
				h.progressLog(w, "Removing key...Failed!")
				hlog(h, "ERROR: Failed to remove key %s", h.keypair.name)
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
			Description: "Assigning network address",
			Func:        allocateElasticIP,
		},
		{
			Description: "Setting workspace's name",
			Func:        associateInstanceName,
		},
		{
			Description: "Waiting for the workspace to be ready",
			Func:        waitForSSH,
			Timeout:     1 * time.Minute,
		},
		{
			Description: "Populating settings",
			Func:        writeSettings,
		},
		{
			Description: "Fetching updates",
			Func:        updateSoftware,
		},
		{
			Description: "Applying settings",
			Func:        restartFreighterd,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			hlog(h, "ERROR: Failed in create host step: %v", err)
			h.logFailed()
			if err == context.DeadlineExceeded {
				return nil, ErrCreateTimeout
			}
			return nil, err
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
			Func:        associateInstanceName,
		},
	}
	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			hlog(h, "ERROR: Failed in update host step: %v", err)
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

	// release elastic IP
	if _, err := h.provider.ReleasePublicAddress(h.instance.PublicIpAddress); err != nil {
		err = fmt.Errorf("amazon: failed to release ElasticIP for %s, err %v", h.instance.InstanceId, err)
		return mh.Error(err, label)
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
	info := &host.Info{
		Provider:       h.DriverName(),
		ID:             h.instance.InstanceId,
		IPAddress:      h.instance.PublicIpAddress,
		FreighterdPort: DefaultPort,
		State:          getState(h.instance),
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
	return strings.Replace(h.notifyURL, "{{slug}}", h.Config.Name, -1)
}

// progressLog writes the message to the given writer and logs the message,
// prefixing host information.  It is used for user-facing messages.  Use
// `hlog` for internal logging.
func (h *Host) progressLog(w io.Writer, format string, v ...interface{}) {
	fmt.Fprintf(w, format, v...)
	hlog(h, format, v...)
}

func (h *Host) logFailed() {
	if h.instance != nil {
		if h.keypair != nil {
			hlog(h, "keyID=%s Leaving instance for inspection: %s", h.keypair.name, h.instance.InstanceId)
		} else {
			hlog(h, "keyID=unknown Leaving instance for inspection: %s", h.instance.InstanceId)
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
	if h.Config.RawRegion != "" {
		return fmt.Sprintf("%s.amazon.%s", prefix, h.Config.RawRegion)
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
		RawRegion: hostConfig.RawRegion,
	}
	auth, err := accessConfig.Auth()
	if err != nil {
		log.Printf("uuid=%s amazon: auth failed %v", defaultConfig["uuid"], err)
		return nil, err
	}
	region, err := accessConfig.Region()
	if err != nil {
		log.Printf("uuid=%s amazon: invalid region %v", defaultConfig["uuid"], err)
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
		uuid:          defaultConfig["uuid"], // this is injected by the server per request
		notifyURL:     defaultConfig["notify_url"],
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

	hlog(h, "inspecting source ami %q", h.Config.SourceAMI)
	imageResp, err := h.provider.Images([]string{h.Config.SourceAMI}, ec2.NewFilter())
	if err != nil {
		return mh.Error(fmt.Errorf("amazon: failed to list images %s", err), label)
	}

	if len(imageResp.Images) == 0 {
		return mh.Error(fmt.Errorf("amazon: image %q not found", h.Config.SourceAMI), label)
	}

	hlog(h, "inspecting source ami %q... Done", h.Config.SourceAMI)
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

	hlog(h, "amazon: creating instance in region %s, size: %s, slug: %s", h.Config.RawRegion, h.Config.InstanceType, h.Config.Name)
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

	return nil
}

// allocateElasticIP creates a new ElasticIP and associate it with the
// instance. This is done so the public IP of the instance will not be changed
// when it's stopped/restarted
func allocateElasticIP(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("allocate_ip")
	defer mh.UpdateSince(label, time.Now())

	allocateResp, err := h.provider.AllocateAddress(&ec2.AllocateAddress{
		Domain: "standard",
	})
	if err != nil {
		return mh.Error(err, label)
	}
	_, err = h.provider.AssociateAddress(&ec2.AssociateAddress{
		InstanceId:         h.instance.InstanceId,
		PublicIp:           allocateResp.PublicIp,
		AllocationId:       allocateResp.AllocationId,
		AllowReassociation: false,
	})
	if err != nil {
		// if failed to associate the ElasticIp with the instance, we remove it
		h.provider.ReleasePublicAddress(allocateResp.PublicIp)
		return mh.Error(err, label)
	}

	// reload the instance to get a new public IP address
	if err := h.reloadInstance(h.instance.InstanceId); err != nil {
		return mh.Error(err, label)
	}

	return nil
}

// associateInstanceName sets the instance's Name tag to the provided name so
// we can later filter this instance using API.
// If this fails, we still consider the operation to be successful
func associateInstanceName(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("set_name")
	defer mh.UpdateSince(label, time.Now())

	tags := []ec2.Tag{{"Name", h.Config.Name}}
	_, err := h.provider.CreateTags([]string{h.instance.InstanceId}, tags)
	if err != nil {
		hlog(h, "Failed to set name for instance %s", h.instance.InstanceId)
	}
	return nil
}

// waitForSSH creates a SSH connection to the host using the temporary keypair
// so we can perform setup actions on it
func waitForSSH(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("ssh_wait")
	defer mh.UpdateSince(label, time.Now())

	hlog(h, "Waiting for SSH to be ready")
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
	hlog(h, "SSH ready for instance %s", h.instance.InstanceId)
	h.conn = sshConn
	return nil
}

// writeSettings uploads necessary file system contents to the host, which
// include package configs, apt details, etc...
func writeSettings(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("write_settings")
	defer mh.UpdateSince(label, time.Now())
	hlog(h, "Writing settings")

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

	hlog(h, "Wrote settings")
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
		if err := h.conn.Execute(fmt.Sprintf("sudo mv %s %s", tmpDest, dest)); err != nil {
			return mh.Error(err, label)
		}
		flist = append(flist, dest)
	}
	return nil
}

// updateSoftware updates the package list and necessary packages that may be
// outdated since the last image builds.
func updateSoftware(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("update_software")
	defer mh.UpdateSince(label, time.Now())
	hlog(h, "Updating freighterd package")

	// TODO move all of cmdParts into a shell script that is written and run
	// (similar to /tmp/already-installed.sh below)
	path := "/tmp/already-installed.sh"
	cmdParts := []string{
		fmt.Sprintf(`{ cat > %s <<- 'EOF'
			#!/bin/sh
			installed_version=$(/usr/sbin/freighterd -v)
			installed_version=${installed_version#'freighterd version '}
			test $installed_version = '%s'
		EOF
		} && chmod +x %s && %s`, path, h.Config.FreighterdVersion, path, path),
		`||`,
		// apt-get update with single source
		// https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=714877#12
		"DEBIAN_FRONTEND=noninteractive sudo apt-get update",
		"-o APT::List-Cleanup=0",
		`-o Dir::Etc::sourceparts="/dev/null"`,
		`-o Dir::Etc::sourcelist="sources.list.d/nitrousio.list"`,
		`&&`,
		"DEBIAN_FRONTEND=noninteractive sudo apt-get install -y",
		`-o Dpkg::Options::="--force-confdef"`, // avoid prompt
		`-o Dpkg::Options::="--force-confold"`, // do not modify existing config file
	}
	pkgspec := "freighterd"
	if h.Config.FreighterdVersion != "" {
		pkgspec = fmt.Sprintf("freighterd=%s", h.Config.FreighterdVersion)
	}
	cmdParts = append(cmdParts, pkgspec)
	cmd := strings.Join(cmdParts, " ")

	if out, err := h.conn.CombinedOutput(cmd); err != nil {
		err = fmt.Errorf("update sofware failed with '%s', output: %s", err, out)
		hlog(h, err.Error())
		if h.Config.FreighterdVersion != "" {
			return mh.Error(err, label)
		}
		// failure to update freighterd when a version is not specified is considered continuable
		return nil
	}

	hlog(h, "Updated freighterd package on %v", h.instance.PublicIpAddress)
	return nil
}

// restartFreighterd restarts the remote freighterd server. This should be run
// after updateSoftware to make sure that it has the latest settings.
func restartFreighterd(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("restart_software")
	defer mh.UpdateSince(label, time.Now())

	hlog(h, "Restarting freighterd")
	if out, err := h.conn.CombinedOutput(`sudo service freighterd restart`); err != nil {
		err = fmt.Errorf("restart freighterd failed with '%s', output: %s", err, out)
		return mh.Error(err, label)
	}
	hlog(h, "Restarted freighterd on %s", h.instance.PublicIpAddress)
	return nil
}

func hlog(h *Host, format string, v ...interface{}) {
	parts := []string{}
	if h.uuid != "" {
		parts = append(parts, fmt.Sprintf("uuid=%s", h.uuid))
	}
	if h.instance != nil {
		parts = append(parts, fmt.Sprintf("instance=%d", h.instance.InstanceId))
	}
	log.Printf(fmt.Sprintf("amazon: %s %s", strings.Join(parts, " "), format), v...)
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
	}
	return state.Unknown
}
