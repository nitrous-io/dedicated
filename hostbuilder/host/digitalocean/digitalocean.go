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

const DefaultPort uint16 = 65333

var (
	TLSCALocation   = "/etc/freighterd/ca.crt"
	TLSCertLocation = "/etc/freighterd/cert.crt"
	TLSKeyLocation  = "/etc/freighterd/key.crt"
	SecretLocation  = "/etc/freighterd/secret"

	DefaultCreateTimeout = 5 * time.Minute
	StateWaitTime        = 3 * time.Second

	ErrCreateTimeout = errors.New("timed out creating host")
	errHostRequired  = errors.New("context requires a host")

	// FIXME: the map should belongs to client, not the builder
	regionsMap = map[string]string{
		"sgp": "sgp1",
		"nyc": "nyc3",
		"sfo": "sfo1",
		"ams": "ams3",
		"lon": "lon1",
	}
)

type tlsConfig struct {
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type HostConfig struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ImageID    string     `json:"image_id"`
	RegionID   string     `json:"region_id"`
	SizeID     string     `json:"size_id"`
	PublicKeys []string   `json:"public_keys"`
	TLS        *tlsConfig `json:"tls"`
	Secret     string     `json:"secret"`

	// for setting up repository on the host
	AptUsername string `json:"apt_username"` // username of nitrous.io
	AptPassword string `json:"apt_password"` // nitrous.io access token
	AptLocation string `json:"apt_location"` // URI of the repository
	AptDist     string `json:"apt_dist"`     // distribution of the repo
}

type Host struct {
	Client *godo.Client
	Config *HostConfig

	droplet       *godo.Droplet
	privateKey    *rsa.PrivateKey
	conn          *sshutil.Conn
	sshKey        *godo.Key
	uuid          string
	createTimeout time.Duration

	cancelCreate chan struct{}
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
			Timeout:     30 * time.Second,
		},
		{
			Description: "Creating workspace",
			Func:        createDroplet,
			Timeout:     16 * time.Minute,
		},
		{
			Description: "Waiting for the workspace to be ready",
			Func:        waitForSSH,
			Timeout:     1 * time.Minute,
		},
		{
			Description: "Writing key",
			Func:        writePublicKeys,
			Timeout:     30 * time.Second,
		},
		{
			Description: "Initializing settings",
			Func:        createEtcFreighterd,
			Timeout:     30 * time.Second,
		},
		{
			Description: "Writing certificates",
			Func:        writeTLSCerts,
			Timeout:     30 * time.Second,
		},
		{
			Description: "Writing token",
			Func:        writeSecret,
			Timeout:     30 * time.Second,
		},
		{
			Description: "Setting up",
			Func:        writeRepository,
			Timeout:     30 * time.Second,
		},
		{
			Description: "Fetching updates",
			Func:        updateSoftware,
			Timeout:     5 * time.Minute,
		},
		{
			Description: "Applying settings",
			Func:        restartFreighterd,
			Timeout:     2 * time.Minute,
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
		hlog(h, "keyID=%d Removing key...", h.sshKey.ID)
		_, err := h.Client.Keys.DeleteByID(h.sshKey.ID)
		if err != nil {
			h.progressLog(w, "Removing key...Failed!")
			hlog(h, "ERROR: Failed to remove key %d", h.sshKey.ID)
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

	hlog(h, "Renaming droplet %d to %s", id, h.Config.Name)
	action, _, err := h.Client.DropletActions.Rename(id, h.Config.Name)
	if err != nil {
		return nil, err
	}
	info := map[string]interface{}{
		"uuid": h.uuid,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := WaitForAction(ctx, h.Client.Actions, action.ID, StateWaitTime, info); err != nil {
		return nil, err
	}
	hlog(h, "Renamed droplet")

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
			allDroplets = append(allDroplets, d)
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
		IPAddress:      h.ipAddress(),
		FreighterdPort: DefaultPort,
		State:          getState(h.droplet),
	}
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

func (h *Host) Restart() (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}
	action, _, err := h.Client.DropletActions.Reboot(id)
	if err != nil {
		return nil, err
	}
	info := map[string]interface{}{
		"uuid": h.uuid,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if _, err = WaitForAction(ctx, h.Client.Actions, action.ID, StateWaitTime, info); err != nil {
		return nil, err
	}

	return h.Info()
}

func (h *Host) DriverName() string {
	return "digitalocean"
}

func (h *Host) id() (int, error) {
	id64, err := strconv.ParseInt(h.Config.ID, 10, 64)
	if err != nil {
		return 0, err
	}
	return int(id64), nil
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
// `hlog` for internal logging.
func (h *Host) progressLog(w io.Writer, format string, v ...interface{}) {
	fmt.Fprintf(w, format, v...)
	hlog(h, format, v...)
}

func (h *Host) logFailedDroplet() {
	if h.droplet != nil {
		if h.sshKey != nil {
			hlog(h, "keyID=%d Leaving droplet for inspection: %d", h.sshKey.ID, h.droplet.ID)
		} else {
			hlog(h, "keyID=unknown Leaving droplet for inspection: %d", h.droplet.ID)
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
		createTimeout: createTimeout,
		cancelCreate:  make(chan struct{}),
	}
	return host, nil
}

func init() {
	host.Hosts["digitalocean"] = NewHost
}

func createSSHKey(ctx context.Context) error {
	hst, ok := host.FromContext(ctx)
	if !ok {
		return errHostRequired
	}
	h := hst.(*Host)
	label := h.metricsLabel("create_key")
	defer mh.UpdateSince(label, time.Now())

	hlog(h, "generating temporary SSH key")
	privateKey, err := sshutil.GenerateSSHKey()
	if err != nil {
		return mh.Error(err, label)
	}
	hlog(h, "generating temporary SSH key... Done")
	h.privateKey = privateKey
	pubKey, err := sshutil.GetPubKey(privateKey)
	if err != nil {
		return mh.Error(err, label)
	}
	hlog(h, "creating SSH keypair on digitalocean")
	sshKey, _, err := h.Client.Keys.Create(&godo.KeyCreateRequest{
		Name:      h.Config.Name,
		PublicKey: string(pubKey),
	})
	if err != nil {
		return mh.Error(err, label+".api")
	}

	h.sshKey = sshKey
	hlog(h, "keyID=%d fingerprint=%s creating SSH keypair on digitalocean... Done", h.sshKey.ID, h.sshKey.Fingerprint)
	return nil
}

func createDroplet(ctx context.Context) error {
	hst, ok := host.FromContext(ctx)
	if !ok {
		return errHostRequired
	}
	h := hst.(*Host)

	defer func(start time.Time) {
		// we are interested in the final context of `h` so label is generated
		// inside defer block
		mh.UpdateSince(h.metricsLabel("create_host"), start)
	}(time.Now())

	label := h.metricsLabel("create_host")
	config := h.Config
	sshKey := h.sshKey

	hlog(h, "digitalocean: creating droplet in region %s, size: %s, slug: %s", config.RegionID, config.SizeID, config.Name)
	normalizedRegion, ok := regionsMap[config.RegionID]
	if !ok {
		err := fmt.Errorf("Invalid region or lookup failure: '%s'", config.RegionID)
		return mh.Error(err, label)
	}
	dropletRoot, _, err := h.Client.Droplets.Create(&godo.DropletCreateRequest{
		Name:   config.Name,
		Region: normalizedRegion,
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
		hlog(h, "[WARNING] digitalocean did not return action data for host %+v", h)
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

func waitForSSH(ctx context.Context) error {
	hst, ok := host.FromContext(ctx)
	if !ok {
		return errHostRequired
	}
	h := hst.(*Host)
	label := h.metricsLabel("ssh_wait")
	defer mh.UpdateSince(label, time.Now())

	hlog(h, "Waiting for SSH to be ready")
	address := fmt.Sprintf("%s:%s", h.ipAddress(), "65100")
	info := map[string]interface{}{
		"uuid": h.uuid,
	}
	connInterval := 5 * time.Second
	sshConn, err := sshutil.WaitForSSH(address, "root", h.privateKey,
		connInterval, ctx.Done(), info)
	if err != nil {
		return mh.Error(err, label)
	}
	hlog(h, "SSH READY for droplet %v", h.droplet.ID)
	h.conn = sshConn
	return nil
}

func writePublicKeys(ctx context.Context) error {
	hst, ok := host.FromContext(ctx)
	if !ok {
		return errHostRequired
	}
	h := hst.(*Host)
	hlog(h, "Writing public key")
	pubKeyR := bytes.NewBuffer(nil)
	for _, k := range h.Config.PublicKeys {
		fmt.Fprintln(pubKeyR, k)
	}
	err := writeFile(h, map[string]io.Reader{
		"/root/.ssh/authorized_keys": pubKeyR,
	})
	hlog(h, "Wrote public key")
	return err
}

func createEtcFreighterd(ctx context.Context) error {
	hst, ok := host.FromContext(ctx)
	if !ok {
		return errHostRequired
	}
	h := hst.(*Host)
	defer mh.UpdateSince(h.metricsLabel("create_config"), time.Now())
	return h.conn.Execute("mkdir -p /etc/freighterd")
}

func writeTLSCerts(ctx context.Context) error {
	hst, ok := host.FromContext(ctx)
	if !ok {
		return errHostRequired
	}
	h := hst.(*Host)
	hlog(h, "Writing TLS certs")
	err := writeFile(h, map[string]io.Reader{
		TLSCALocation:   bytes.NewBufferString(h.Config.TLS.CA),
		TLSCertLocation: bytes.NewBufferString(h.Config.TLS.Cert),
		TLSKeyLocation:  bytes.NewBufferString(h.Config.TLS.Key),
	})
	hlog(h, "Wrote TLS certs")
	return err
}

func writeSecret(ctx context.Context) error {
	hst, ok := host.FromContext(ctx)
	if !ok {
		return errHostRequired
	}
	h := hst.(*Host)
	hlog(h, "Writing host secret")
	err := writeFile(h, map[string]io.Reader{
		SecretLocation: bytes.NewBufferString(h.Config.Secret),
	})
	hlog(h, "Wrote host secret")
	return err
}

func writeFile(h *Host, files map[string]io.Reader) error {
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

func writeRepository(ctx context.Context) error {
	hst, ok := host.FromContext(ctx)
	if !ok {
		return errHostRequired
	}
	h := hst.(*Host)
	hlog(h, "Uploading apt details")
	buf := bytes.NewBuffer(nil)
	if err := host.SourceTemplate.Execute(buf, h.Config); err != nil {
		return err
	}
	if err := writeFile(h, map[string]io.Reader{host.SourceLocation: buf}); err != nil {
		return err
	}
	hlog(h, "Uploaded apt details at %v", h.ipAddress())
	return nil
}

func updateSoftware(ctx context.Context) error {
	hst, ok := host.FromContext(ctx)
	if !ok {
		return errHostRequired
	}
	h := hst.(*Host)
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
		"freighterd",
	}
	cmd := strings.Join(cmdParts, " ")

	if out, err := h.conn.CombinedOutput(cmd); err != nil {
		e := fmt.Errorf("update sofware failed with '%s', output: %s", err, out)
		return mh.Error(e, label)
	}
	hlog(h, "Updated freighterd package on %v", h.ipAddress())
	return nil
}

func restartFreighterd(ctx context.Context) error {
	hst, ok := host.FromContext(ctx)
	if !ok {
		return errHostRequired
	}
	h := hst.(*Host)
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
