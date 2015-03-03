package amazon

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"golang.org/x/net/context"
)

const (
	DefaultPort uint16 = 65333

	sshPort uint16 = 65100
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
	StateWaitTime        = 3 * time.Second

	ErrDisabledEnhancedNetworking = errors.New("disabled enhanced networking")
	ErrCreateTimeout              = errors.New("timed out creating host")
)

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
	RawSecurityGroup map[string]string `json:"security_group"`

	// should be enabled if the underlying SourceAMI is HVM
	EnhancedNetworking bool `json:"enhanced_networking"`

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

// securityGroups returns a list of preconfigured security groups that we are
// going to use for all the instances.
func (h *HostConfig) securityGroups() []ec2.SecurityGroup {
	return []ec2.SecurityGroup{
		ec2.SecurityGroup{
			Id:   h.RawSecurityGroup["id"],
			Name: h.RawSecurityGroup["name"],
		},
	}
}

type Host struct {
	Auth   aws.Auth
	Config *HostConfig

	provider      *ec2.EC2
	instance      *ec2.Instance
	createTimeout time.Duration

	// the following settings are only available during the creation phase
	// where we stored the temporary keypair to access the instance
	keypair *keypair      // private keypair of the instance
	conn    *sshutil.Conn // ssh connection to the instance
}

func (h *Host) Create(w io.Writer) (*host.Info, error) {
	// generate the temporary keypair that is used to communicate with the host
	k, err := generateTempKeypair(h.provider)
	if err != nil {
		return nil, err
	}
	defer k.Delete()
	h.keypair = k

	// ctx is the Context for this method. Calling cancel closes the ctx.Done
	// channel, which will in turn signal for child requests started by this
	// method to be cancelled.
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Store host and writer in ctx for child requests.
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
			h.logFailedDroplet()
			if err == context.DeadlineExceeded {
				return nil, ErrCreateTimeout
			}
			return nil, err
		}
	}

	if h.sshKey != nil {
		fmt.Fprintf(w, "Removing key...")
		hlog(h, "keyID=%d Removing key...", h.keypair.name)
		if err := h.keypair.Delete(); err != nil {
			h.progressLog(w, "Removing key...Failed!")
			hlog(h, "ERROR: Failed to remove key %d", h.keypair.name)
		} else {
			h.progressLog(w, "Removing key...Done!")
		}
	}

	info, err := h.Info()
	h.progressLog(w, "Provisioning host...Done!")
	return info, err
}

func (h *Host) Update(w io.Writer) (*host.Info, error) {
	return nil, errors.New("amazon: Update() not implemented because we don't care")
}

func (h *Host) Destroy() error {
	instancesResp, err := h.provider.Instances([]string{h.Config.ID}, nil)
	if err != nil {
		return err
	}
	// if the instance with specified ID does not exist. them we consider the
	// Destroy operation to be successful
	if len(instancesResp.Reservations) == 0 || len(instancesResp.Reservations[0].Instances) == 0 {
		return nil
	}
	instance := instancesResp.Reservations[0].Instances[0]
	if _, err := h.provider.TerminateInstances([]string{instance.InstanceId}); err != nil {
		return fmt.Errorf("amazon: failed to terminate instance %s, err %v", instance.InstanceId, err)
	}
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
		log.Printf("uuid=%s amazon: invalid config %s", defaultConfig["uuid"], config)
		return nil, err
	}
	// get authentication
	auth, err := getAuth(defaultConfig["access_key"], defaultConfig["secret_key"])
	if err != nil {
		log.Printf("uuid=%s amazon: auth failed %v", defaultConfig["uuid"], config)
		return nil, err
	}
	// get create timeout
	createTimeout := DefaultCreateTimeout
	d, err := time.ParseDuration(defaultConfig["create_timeout"])
	if err == nil {
		createTimeout = d
	}
	host := &Host{
		Auth:          auth,
		Config:        hostConfig,
		provider:      ec2.New(auth, region),
		uuid:          defaultConfig["uuid"],
		notifyURL:     defaultConfig["notify_url"],
		createTimeout: createTimeout,
	}
	return host, nil
}

// progressLog writes the message to the given writer and logs the message,
// prefixing host information.  It is used for user-facing messages.  Use
// `hlog` for internal logging.
func (h *Host) progressLog(w io.Writer, format string, v ...interface{}) {
	fmt.Fprintf(w, format, v...)
	hlog(h, format, v...)
}

func (h *Host) logFailedDroplet() {
	if h.instance != nil {
		if h.keypair != nil {
			hlog(h, "keyID=%s Leaving instance for inspection: %s", h.keypair.name, h.instance.InstanceId)
		} else {
			hlog(h, "keyID=unknown Leaving instance for inspection: %s", h.instance.InstanceId)
		}
	}
}

func init() {
	host.Hosts["amazon"] = NewHost
}

func inspectSourceAMI(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)

	imageResp, err := h.provider.Images([]string{h.Config.SourceAMI}, ec2.NewFilter())
	if err != nil {
		return fmt.Errorf("inspectSourceAMI: failed to list images %s", err)
	}

	if len(imageResp.Images) == 0 {
		return fmt.Errorf("inspectSourceAMI: %q not found", h.Config.SourceAMI)
	}

	image := &imageResp.Images[0]
	if h.Config.EnhancedNetworking && image.VirtualizationType != "hvm" {
		return ErrDisabledEnhancedNetworking
	}

	return nil
}

func createInstance(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)

	runOpts := &ec2.RunInstances{
		KeyName:        h.keypair.name,
		ImageId:        h.Config.SourceAMI,
		InstanceType:   h.Config.InstanceType,
		SecurityGroups: h.Config.securityGroups(),
		AvailZone:      h.Config.AvailabilityZone,
		MinCount:       0,
		MaxCount:       0,
	}

	// create the instance now
	resp, err := h.provider.RunInstances(runOpts)
	if err != nil {
		return fmt.Errorf("amazon: failed to create new instance %s", err)
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
		return fmt.Errorf("amazon: failed to change state %s", err)
	}
	h.instance = updatedInstance.(*ec2.Instance)

	// set name of the instance. If this fails, we still consider the operation
	// to be successful
	ec2Tags = []ec2.Tag{{"Name", h.Config.Name}}
	if _, err := h.provider.CreateTags([]string{s.instance.InstanceId}, ec2Tags); err != nil {
		hlog(h, "amazon: Failed to set name for instance %s", s.instance.InstanceID)
	}

	return nil
}

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
	sshConn, err := sshutil.WaitForSSH(address, "root", h.keypair.privateKey,
		connInterval, ctx.Done(), info)
	if err != nil {
		return mh.Error(err, label)
	}
	hlog(h, "SSH ready for droplet %v", h.droplet.ID)
	h.conn = sshConn
	return nil
}

func writeSettings(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	hlog(h, "Writing settings")

	pubKeysBuf := bytes.NewBuffer(nil)
	for _, k := range h.Config.PublicKeys {
		fmt.Fprintln(pubKeysBuf, k)
	}

	aptSourcesBuf := bytes.NewBuffer(nil)
	if err := host.SourceTemplate.Execute(aptSourcesBuf, h.Config); err != nil {
		return err
	}

	cfg := h.configFile()
	cfgb, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("unable to marshal config JSON, err: %v", err)
	}

	if err := h.conn.Execute("mkdir -p /etc/freighterd"); err != nil {
		return err
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

func uploadFile(h *Host, files map[string]io.Reader) error {
	label := h.metricsLabel("write_file")
	defer mh.UpdateSince(label, time.Now())
	flist := []string{}
	for dest, val := range files {
		if err := h.conn.Upload(dest, val); err != nil {
			return mh.Error(err, label)
		}
		flist = append(flist, dest)
	}
	hlog(h, "Uploaded (%s) to %v", strings.Join(flist, " "), h.ipAddress())
	return nil
}

func updateSoftware(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("update_software")
	defer mh.UpdateSince(label, time.Now())

	hlog(h, "Updating freighterd package")

	cmdParts := []string{
		// apt-get update with single source
		// https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=714877#12
		"DEBIAN_FRONTEND=noninteractive apt-get update",
		"-o APT::List-Cleanup=0",
		`-o Dir::Etc::sourceparts="/dev/null"`,
		`-o Dir::Etc::sourcelist="sources.list.d/nitrousio.list"`,
		`&&`,
		"DEBIAN_FRONTEND=noninteractive apt-get install -y",
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
		e := fmt.Errorf("update sofware failed with '%s', output: %s", err, out)
		hlog(h, e.Error())
		mh.Error(e, label)
		// failure to update a host is considered continuable
		return nil
	}
	hlog(h, "Updated freighterd package on %v", h.ipAddress())
	return nil
}

func restartFreighterd(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("restart_software")
	defer mh.UpdateSince(label, time.Now())

	hlog(h, "Restarting freighterd")
	if out, err := h.conn.CombinedOutput(`service freighterd restart`); err != nil {
		e := fmt.Errorf("restart freighterd failed with '%s', output: %s", err, out)
		return mh.Error(e, label)
	}
	hlog(h, "Restarted freighterd on %v", h.ipAddress())
	return nil
}

func hlog(h *Host, format string, v ...interface{}) {
	parts := []string{}
	if h.uuid != "" {
		parts = append(parts, fmt.Sprintf("uuid=%s", h.uuid))
	}
	if h.droplet != nil {
		parts = append(parts, fmt.Sprintf("droplet=%d", h.droplet.ID))
	}
	log.Printf(fmt.Sprintf("%s %s", strings.Join(parts, " "), format), v...)
}

// getAuth returns a valid aws.Auth object for access to AWS services, or
// an error if the authentication couldn't be resolved.
func getAuth(access, secret string) (aws.Auth, error) {
	return aws.GetAuth(access, secret)
}
