package digitalocean

import (
	"bytes"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/goauth2/oauth"

	"github.com/digitalocean/godo"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
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

	ErrCreateTimeout = errors.New("timed out creating host")

	// this pattern is used to match droplets used to host tender,
	// we are filtering them from the hosts list.
	tenderDropletNameRegex *regexp.Regexp = regexp.MustCompile(`tender.[A-Za-z0-9\-]+.com`)
)

type tlsConfig struct {
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type HostConfig struct {
	ID         string     `json:"id"`
	HostID     int64      `json:"host_id"` // database id of the host
	Name       string     `json:"name"`
	ImageID    string     `json:"image_id"`
	RegionID   string     `json:"region_id"`
	SizeID     string     `json:"size_id"`
	PublicKeys []string   `json:"public_keys"`
	TLS        *tlsConfig `json:"tls"`
	Secret     string     `json:"secret"`

	// for setting up repository on the host
	AptUsername string `json:"apt_username"` // host id on nitrousd
	AptPassword string `json:"apt_password"` // host secret
	AptLocation string `json:"apt_location"` // URI of the repository
	AptDist     string `json:"apt_dist"`     // distribution of the repo

	FreighterdVersion string `json:"freighterd_version"`
}

type Host struct {
	Client *godo.Client
	Config *HostConfig

	droplet       *godo.Droplet
	privateKey    *rsa.PrivateKey
	conn          *sshutil.Conn
	sshKey        *godo.Key
	uuid          string
	notifyURL     string
	createTimeout time.Duration
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

	// Store host and writer in ctx for child requests.
	hctx := host.NewContext(ctx, h, w)

	steps := []host.Step{
		{
			Description: "Creating key",
			Func:        createSSHKey,
		},
		{
			Description: "Creating workspace",
			Func:        createDroplet,
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
			h.Logf("ERROR: Failed in create host step: %v", err)
			h.logFailedDroplet()
			if err == context.DeadlineExceeded {
				return nil, ErrCreateTimeout
			}
			return nil, err
		}
	}

	if h.sshKey != nil {
		fmt.Fprintf(w, "Removing key...")
		h.Logf("keyID=%d Removing key...", h.sshKey.ID)
		_, err := h.Client.Keys.DeleteByID(h.sshKey.ID)
		if err != nil {
			h.progressLog(w, "Removing key...Failed!")
			h.Logf("ERROR: Failed to remove key %d", h.sshKey.ID)
		}
		h.progressLog(w, "Removing key...Done!")
	}

	info := h.dropletToInfo()
	h.progressLog(w, "Provisioning host...Done!")
	return info, nil
}

func (h *Host) Update(w io.Writer) (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}

	h.Logf("Renaming droplet %d to %s", id, h.Config.Name)
	action, _, err := h.Client.DropletActions.Rename(id, h.Config.Name)
	if err != nil {
		return nil, err
	}

	if err := h.waitForAction(action.ID, 30*time.Second); err != nil {
		return nil, err
	}
	h.Logf("Renamed droplet")

	h.progressLog(w, "Updating host...Done!")
	return h.Info()
}

func (h *Host) Destroy() error {
	id, err := h.id()
	if err != nil {
		return err
	}
	dropletRoot, resp, err := h.Client.Droplets.Get(id)
	if err != nil {
		// if the droplet does not exist, then consider that it is sucessfully
		// destroyed
		// https://github.com/nitrous-io/nitro/issues/99
		if resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	h.droplet = dropletRoot.Droplet
	if resp, err := h.Client.Droplets.Delete(id); err != nil {
		// if the droplet does not exist, then consider that it is sucessfully
		// destroyed
		// https://github.com/nitrous-io/nitro/issues/99
		if resp.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	return nil
}

func (h *Host) Info() (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}
	dropletRoot, _, err := h.Client.Droplets.Get(id)
	if err != nil {
		return nil, err
	}
	h.droplet = dropletRoot.Droplet
	return h.dropletToInfo(), nil
}

func (h *Host) List() ([]*host.Info, error) {
	allDroplets := []godo.Droplet{}
	opt := &godo.ListOptions{}

	// Collect all droplets, taking pagination into account.
	for {
		droplets, resp, err := h.Client.Droplets.List(opt)
		if err != nil {
			return nil, err
		}

		for _, d := range droplets {
			if !isTenderDroplet(d) {
				allDroplets = append(allDroplets, d)
			}
		}

		if resp.Links == nil || resp.Links.IsLastPage() {
			break
		}

		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}

		opt.Page = page + 1
	}

	list := make([]*host.Info, 0, len(allDroplets))
	for _, droplet := range allDroplets {
		list = append(list, &host.Info{
			Provider:       h.DriverName(),
			ID:             strconv.Itoa(droplet.ID),
			Region:         droplet.Region.Slug,
			Name:           droplet.Name,
			IPAddress:      droplet.Networks.V4[0].IPAddress,
			FreighterdPort: DefaultPort,
			State:          getState(&droplet),
		})
	}
	return list, nil
}

func (h *Host) dropletToInfo() *host.Info {
	return &host.Info{
		Provider:       h.DriverName(),
		ID:             strconv.Itoa(h.droplet.ID),
		Region:         h.droplet.Region.Slug,
		IPAddress:      h.ipAddress(),
		FreighterdPort: DefaultPort,
		State:          getState(h.droplet),
	}
}

func isTenderDroplet(droplet godo.Droplet) bool {
	return tenderDropletNameRegex.MatchString(droplet.Name)
}

func getState(droplet *godo.Droplet) state.State {
	// TODO DigitalOcean's API only returns "new", "active", "off" or "archive".
	// If we need "stopping", we will probably need to retrieve the actions for
	// the droplet.
	switch droplet.Status {
	case "new":
		return state.Starting
	case "active":
		return state.Running
	case "off":
		return state.Stopped
	}
	return state.Unknown
}

func (h *Host) Stop() (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}

	action, _, err := h.Client.DropletActions.PowerOff(id)
	if err != nil {
		return nil, err
	}

	if err := h.waitForAction(action.ID, 10*time.Minute); err != nil {
		return nil, err
	}

	return h.Info()
}

func (h *Host) Start() (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}

	action, _, err := h.Client.DropletActions.PowerOn(id)
	if err != nil {
		return nil, err
	}

	if err := h.waitForAction(action.ID, 10*time.Minute); err != nil {
		return nil, err
	}

	return h.Info()
}

func (h *Host) Restart() (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}
	action, _, err := h.Client.DropletActions.Reboot(id)
	if err != nil {
		return nil, err
	}

	if err := h.waitForAction(action.ID, 15*time.Minute); err != nil {
		return nil, err
	}

	return h.Info()
}

func (h *Host) DriverName() string {
	return "digitalocean"
}

// Logf logs the data together with the host uuid and droplet id if they are
// avalaible
func (h *Host) Logf(format string, v ...interface{}) {
	parts := []string{}
	if h.uuid != "" {
		parts = append(parts, fmt.Sprintf("uuid=%s", h.uuid))
	}
	if h.droplet != nil {
		parts = append(parts, fmt.Sprintf("droplet=%d", h.droplet.ID))
	}
	log.Printf(fmt.Sprintf("%s %s", strings.Join(parts, " "), format), v...)
}

// MetricsLabel returns the label with the current host's region if possible
func (h *Host) MetricsLabel(prefix string) string {
	var region string
	if h.droplet != nil && h.droplet.Region != nil {
		region = h.droplet.Region.Slug
	}
	if region != "" {
		return fmt.Sprintf("%s.digitalocean.%s", prefix, region)
	}
	return fmt.Sprintf("%s.digitalocean", prefix)
}

func (h *Host) id() (int, error) {
	id64, err := strconv.ParseInt(h.Config.ID, 10, 64)
	if err != nil {
		return 0, err
	}
	return int(id64), nil
}

func (h *Host) waitForAction(id int, timeout time.Duration) error {
	info := map[string]interface{}{
		"uuid": h.uuid,
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if _, err := WaitForAction(ctx, h.Client.Actions, id, StateWaitTime, info); err != nil {
		return err
	}
	return nil
}

// ipAddress get the ipv4 address of the droplet. DigitalOcean always returns
// one IPv4 for a valid container. The format of Networks is:
// "networks": {
//    "v4": [
//      {
//        "ip_address": "127.0.0.1",
//        "netmask": "255.255.255.0",
//        "gateway": "127.0.0.2",
//        "type": "public"
//      }
//    ],
//  }
//
//  We are not allocating/using IPv6 yet so that's empty
func (h *Host) ipAddress() string {
	return h.droplet.Networks.V4[0].IPAddress
}

// progressLog writes the message to the given writer and logs the message,
// prefixing host information.  It is used for user-facing messages.  Use
// `h.Logf` for internal logging.
func (h *Host) progressLog(w io.Writer, format string, v ...interface{}) {
	fmt.Fprintf(w, format, v...)
	h.Logf(format, v...)
}

func (h *Host) logFailedDroplet() {
	if h.droplet != nil {
		if h.sshKey != nil {
			h.Logf("keyID=%d Leaving droplet for inspection: %d", h.sshKey.ID, h.droplet.ID)
		} else {
			h.Logf("keyID=unknown Leaving droplet for inspection: %d", h.droplet.ID)
		}
	}
}

// mlabel returns provider.region which is probably helpful if we ever want to
// differentiate between regions.
//
// The result will have a form of prefix.digitalocean<.region>
func (h *Host) metricsLabel(prefix string) string {
	var region string
	if h.droplet != nil && h.droplet.Region != nil {
		region = h.droplet.Region.Slug
	}
	if region != "" {
		return fmt.Sprintf("%s.digitalocean.%s", prefix, region)
	}
	return fmt.Sprintf("%s.digitalocean", prefix)
}

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

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		log.Printf("uuid=%s digitalocean: invalid config %s", defaultConfig["uuid"], config)
		return nil, err
	}
	if defaultConfig["token"] == "" {
		return nil, errors.New("digitalocean builder requires valid access token")
	}
	createTimeout := DefaultCreateTimeout
	d, err := time.ParseDuration(defaultConfig["create_timeout"])
	if err == nil {
		createTimeout = d
	}
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: defaultConfig["token"]},
	}
	client := godo.NewClient(t.Client())
	host := &Host{
		Config:        hostConfig,
		Client:        client,
		uuid:          defaultConfig["uuid"],
		notifyURL:     defaultConfig["notify_url"],
		createTimeout: createTimeout,
	}
	return host, nil
}

func init() {
	host.Hosts["digitalocean"] = NewHost

}

func createSSHKey(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("create_key")
	defer mh.UpdateSince(label, time.Now())

	h.Logf("generating temporary SSH key")
	privateKey, err := sshutil.GenerateSSHKey()
	if err != nil {
		return mh.Error(err, label)
	}
	h.Logf("generating temporary SSH key... Done")
	h.privateKey = privateKey
	pubKey, err := sshutil.GetPubKey(privateKey)
	if err != nil {
		return mh.Error(err, label)
	}
	h.Logf("creating SSH keypair on digitalocean")
	sshKey, _, err := h.Client.Keys.Create(&godo.KeyCreateRequest{
		Name:      h.Config.Name,
		PublicKey: string(pubKey),
	})
	if err != nil {
		return mh.Error(err, label+".api")
	}

	h.sshKey = sshKey
	h.Logf("keyID=%d fingerprint=%s creating SSH keypair on digitalocean... Done", h.sshKey.ID, h.sshKey.Fingerprint)
	return nil
}

func createDroplet(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)

	defer func(start time.Time) {
		// we are interested in the final context of `h` so label is generated
		// inside defer block
		mh.UpdateSince(h.metricsLabel("create_host"), start)
	}(time.Now())

	label := h.metricsLabel("create_host")
	config := h.Config
	sshKey := h.sshKey

	h.Logf("digitalocean: creating droplet in region %s, size: %s, slug: %s", config.RegionID, config.SizeID, config.Name)
	dropletRoot, _, err := h.Client.Droplets.Create(&godo.DropletCreateRequest{
		Name:   config.Name,
		Region: config.RegionID,
		Size:   config.SizeID,
		Image:  config.ImageID,
		SSHKeys: []interface{}{
			sshKey.ID,
		},
	})
	if err != nil {
		return mh.Error(err, label)
	}
	h.droplet = dropletRoot.Droplet

	if len(dropletRoot.Links.Actions) > 0 {
		info := map[string]interface{}{
			"uuid": h.uuid,
		}
		la := dropletRoot.Links.Actions[0]

		ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()
		if _, err := WaitForAction(ctx, h.Client.Actions, la.ID, StateWaitTime, info); err != nil {
			return mh.Error(err, label)
		}
	} else {
		h.Logf("[WARNING] digitalocean did not return action data for host %+v", h)
		return nil
	}

	dropletRoot, _, err = h.Client.Droplets.Get(dropletRoot.Droplet.ID)
	if err != nil {
		err = fmt.Errorf("create: failed to get droplet after it was successfully created %s", err)
		return mh.Error(err, label)
	}

	h.droplet = dropletRoot.Droplet
	return nil
}

func waitForSSH(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.metricsLabel("ssh_wait")
	defer mh.UpdateSince(label, time.Now())

	h.Logf("Waiting for SSH to be ready")
	address := fmt.Sprintf("%s:%d", h.ipAddress(), sshPort)
	info := map[string]interface{}{
		"uuid": h.uuid,
	}
	connInterval := 5 * time.Second
	sshConn, err := sshutil.WaitForSSH(address, "root", h.privateKey,
		connInterval, ctx.Done(), info)
	if err != nil {
		return mh.Error(err, label)
	}
	h.Logf("SSH ready for droplet %v", h.droplet.ID)
	h.conn = sshConn
	return nil
}

func writeSettings(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	h.Logf("Writing settings")

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

	h.Logf("Wrote settings")
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
	h.Logf("Uploaded (%s) to %v", strings.Join(flist, " "), h.ipAddress())
	return nil
}
