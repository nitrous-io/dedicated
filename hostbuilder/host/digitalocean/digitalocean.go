package digitalocean

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"code.google.com/p/goauth2/oauth"

	"github.com/Sirupsen/logrus"
	"github.com/digitalocean/godo"
	"github.com/nitrous-io/dedicated/event"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	mh "github.com/nitrous-io/dedicated/pkg/metricshelper"
	"github.com/nitrous-io/dedicated/pkg/queue"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
	"golang.org/x/net/context"
)

var (
	DefaultCreateTimeout = 10 * time.Minute
	StateWaitTime        = 3 * time.Second

	ErrCreateTimeout = errors.New("timed out creating host")

	// this pattern is used to match droplets used to host tender,
	// we are filtering them from the hosts list.
	tenderDropletNameRegex *regexp.Regexp = regexp.MustCompile(`tender.[A-Za-z0-9\-]+.com`)
)

type HostConfig struct {
	ID         string          `json:"id"`
	HostID     int64           `json:"host_id"` // database id of the host
	Name       string          `json:"name"`
	ImageID    string          `json:"image_id"`
	RegionID   string          `json:"region_id"`
	SizeID     string          `json:"size_id"`
	PublicKeys []string        `json:"public_keys"`
	TLS        *host.TLSConfig `json:"tls"`
	Secret     string          `json:"secret"`

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
	sshKey        *godo.Key
	uuid          string
	notifyURL     string
	createTimeout time.Duration
}

func (h *Host) CreateWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	ctx, cancel = context.WithTimeout(context.Background(), h.createTimeout)
	defer cancel()

	hctx := host.NewContext(ctx, h, h, nil)

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
			Func:        host.WaitForSSH,
			Timeout:     1 * time.Minute,
		},
		{
			Description: "Populating settings",
			Func:        host.WriteSettings,
		},
		{
			Description: "Fetching updates",
			Func:        host.UpdateSoftware,
		},
		{
			Description: "Applying settings",
			Func:        host.RestartServices,
		},
	}

	for _, step := range steps {
		if err := hctx.Run(step); err != nil {
			errorMsg := fmt.Sprintf("ERROR: Failed in create host step: %v", err)
			h.Printf(errorMsg)
			h.logFailedDroplet()
			host.PublishMessage(qp, event.HostCreateFailed, h.uuid, queueInfo, errorMsg, queue.Create)
			if err == context.DeadlineExceeded {
				return nil, ErrCreateTimeout
			}
			return nil, err
		}

		host.PublishMessage(qp, event.HostCreating, h.uuid, queueInfo, step.Description, queue.Create)
	}

	if h.sshKey != nil {
		host.PublishMessage(qp, event.HostCreating, h.uuid, queueInfo, "Removing key...", queue.Create)
		h.Printf("keyID=%d Removing key...", h.sshKey.ID)
		_, err := h.Client.Keys.DeleteByID(h.sshKey.ID)
		if err != nil {
			host.PublishMessage(qp, event.HostCreating, h.uuid, queueInfo, "Removing key...Failed!", queue.Create)
			h.Printf("ERROR: Failed to remove key %d", h.sshKey.ID)
		}
		host.PublishMessage(qp, event.HostCreating, h.uuid, queueInfo, "Removing key...Done!", queue.Create)
	}

	info := h.dropletToInfo()
	host.PublishHost(qp, event.HostCreated, h.uuid, queueInfo, info, queue.Create, "")
	return info, nil
}

func (h *Host) Update(w io.Writer) (*host.Info, error) {
	id, err := h.id()
	if err != nil {
		return nil, err
	}

	h.Printf("Renaming droplet %d to %s", id, h.Config.Name)
	action, _, err := h.Client.DropletActions.Rename(id, h.Config.Name)
	if err != nil {
		return nil, err
	}

	if err := h.waitForAction(action.ID, 30*time.Second); err != nil {
		return nil, err
	}
	h.Printf("Renamed droplet")

	h.progressLog(w, "Updating host...Done!")
	return h.Info()
}

func (h *Host) RespecWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
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
			FreighterdPort: host.DefaultPort,
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
		FreighterdPort: host.DefaultPort,
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

func (h *Host) StopWithQueue(qp queue.Publisher, queueInfo *host.QueueInfo) (*host.Info, error) {
	host.PublishMessage(qp, event.HostStopping, h.uuid, queueInfo, "Stopping host", queue.Stop)

	id, err := h.id()
	if err != nil {
		if info, hostInfoErr := h.Info(); hostInfoErr == nil {
			host.PublishHost(qp, event.HostStopFailed, h.uuid, queueInfo, info, queue.Stop, err.Error())
		} else {
			logrus.Debugf("uuid=%s digitalocean: failed to get host info: %v", h.uuid, hostInfoErr)
		}
		return nil, err
	}

	action, _, err := h.Client.DropletActions.PowerOff(id)
	if err != nil {
		if info, hostInfoErr := h.Info(); hostInfoErr == nil {
			host.PublishHost(qp, event.HostStopFailed, h.uuid, queueInfo, info, queue.Stop, err.Error())
		} else {
			logrus.Debugf("uuid=%s digitalocean: failed to get host info: %v", h.uuid, hostInfoErr)
		}
		return nil, err
	}

	if err := h.waitForAction(action.ID, 10*time.Minute); err != nil {
		if info, hostInfoErr := h.Info(); hostInfoErr == nil {
			host.PublishHost(qp, event.HostStopFailed, h.uuid, queueInfo, info, queue.Stop, err.Error())
		} else {
			logrus.Debugf("uuid=%s digitalocean: failed to get host info: %v", h.uuid, hostInfoErr)
		}
		return nil, err
	}

	info, err := h.Info()
	if err == nil {
		host.PublishHost(qp, event.HostStopped, h.uuid, queueInfo, info, queue.Stop, "")
	}

	return info, err
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

// Printf logs the data together with the host uuid and droplet id if they are
// avalaible
func (h *Host) Printf(format string, v ...interface{}) {
	parts := []string{}
	if h.uuid != "" {
		parts = append(parts, fmt.Sprintf("uuid=%s", h.uuid))
	}
	if h.droplet != nil {
		parts = append(parts, fmt.Sprintf("droplet=%d", h.droplet.ID))
	}
	logrus.Infof(fmt.Sprintf("%s %s", strings.Join(parts, " "), format), v...)
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

// Settings returns the dynamic settings of the host
func (h *Host) Settings() *host.Settings {
	return host.NewSettings(
		h.Config.TLS,
		&host.AptConfig{
			AptUsername: h.Config.AptUsername,
			AptPassword: h.Config.AptPassword,
			AptLocation: h.Config.AptLocation,
			AptDist:     h.Config.AptDist,
		},
		"/root",
		h.generateNotifyURL(),
		h.Config.Secret,
		h.Config.FreighterdVersion,
		h.Config.PublicKeys,
	)
}

// SSHConfig returns the host's SSH confiration. This configuration is only
// valid during creation phase.
func (h *Host) SSHConfig() *host.SSHConfig {
	return &host.SSHConfig{
		IPAddress:  h.ipAddress(),
		Port:       host.SSHPort,
		Username:   "root",
		PrivateKey: h.privateKey,
	}
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
// `h.Printf` for internal logging.
func (h *Host) progressLog(w io.Writer, format string, v ...interface{}) {
	fmt.Fprintf(w, format, v...)
	h.Printf(format, v...)
}

func (h *Host) logFailedDroplet() {
	if h.droplet != nil {
		if h.sshKey != nil {
			h.Printf("keyID=%d Leaving droplet for inspection: %d", h.sshKey.ID, h.droplet.ID)
		} else {
			h.Printf("keyID=unknown Leaving droplet for inspection: %d", h.droplet.ID)
		}
	}
}

func (h *Host) generateNotifyURL() string {
	return strings.Replace(h.notifyURL, "{{slug}}", strconv.FormatInt(h.Config.HostID, 10), -1)
}

func New(driverOptions map[string]string, config string) (host.Host, error) {
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		logrus.Errorf("uuid=%s digitalocean: invalid config %s", driverOptions["uuid"], config)
		return nil, err
	}
	if driverOptions["token"] == "" {
		return nil, errors.New("digitalocean builder requires valid access token")
	}
	createTimeout := DefaultCreateTimeout
	d, err := time.ParseDuration(driverOptions["create_timeout"])
	if err == nil {
		createTimeout = d
	}
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: driverOptions["token"]},
	}
	client := godo.NewClient(t.Client())
	host := &Host{
		Config:        hostConfig,
		Client:        client,
		uuid:          driverOptions["uuid"],
		notifyURL:     driverOptions["notify_url"],
		createTimeout: createTimeout,
	}
	return host, nil
}

func init() {
	host.Register("digitalocean", &host.RegisteredDriver{
		New: New,
	})
}

func createSSHKey(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	label := h.MetricsLabel("create_key")
	defer mh.UpdateSince(label, time.Now())

	h.Printf("generating temporary SSH key")
	privateKey, err := sshutil.GenerateSSHKey()
	if err != nil {
		return mh.Error(err, label)
	}
	h.Printf("generating temporary SSH key... Done")
	h.privateKey = privateKey
	pubKey, err := sshutil.GetPubKey(privateKey)
	if err != nil {
		return mh.Error(err, label)
	}
	h.Printf("creating SSH keypair on digitalocean")
	sshKey, _, err := h.Client.Keys.Create(&godo.KeyCreateRequest{
		Name:      h.Config.Name,
		PublicKey: string(pubKey),
	})
	if err != nil {
		return mh.Error(err, label+".api")
	}

	h.sshKey = sshKey
	h.Printf("keyID=%d fingerprint=%s creating SSH keypair on digitalocean... Done", h.sshKey.ID, h.sshKey.Fingerprint)
	return nil
}

func createDroplet(ctx *host.HostContext) error {
	h := ctx.Host.(*Host)
	defer func(start time.Time) {
		// we are interested in the final context of `h` so label is generated
		// inside defer block
		mh.UpdateSince(h.MetricsLabel("create_host"), start)
	}(time.Now())

	label := h.MetricsLabel("create_host")
	config := h.Config
	sshKey := h.sshKey

	h.Printf("digitalocean: creating droplet in region %s, size: %s, slug: %s", config.RegionID, config.SizeID, config.Name)
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
		h.Printf("[WARNING] digitalocean did not return action data for host %+v", h)
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
