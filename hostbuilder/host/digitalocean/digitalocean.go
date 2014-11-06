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
	"github.com/nitrous-io/dedicated/hostbuilder/context"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
)

var (
	StateWaitTime = 3 * time.Second

	// FIXME: the map should belongs to client, not the builder
	regionsMap = map[string]string{
		"sgp": "sgp1",
		"nyc": "nyc3",
		"sfo": "sfo1",
		"ams": "ams3",
		"lon": "lon1",
	}
)

type HostConfig struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ImageID    string   `json:"image_id"`
	RegionID   string   `json:"region_id"`
	SizeID     string   `json:"size_id"`
	PublicKeys []string `json:"public_keys"`
	Secret     string   `json:"secret"`

	// tls data that needs to be saved on the host
	TLS *host.TLSConfig `json:"tls"`

	// for setting up repository on the host
	AptUsername string `json:"apt_username"` // username of nitrous.io
	AptPassword string `json:"apt_password"` // nitrous.io access token
	AptLocation string `json:"apt_location"` // URI of the repository
	AptDist     string `json:"apt_dist"`     // distribution of the repo
}

type Host struct {
	Client *godo.Client
	Config *HostConfig

	droplet    *godo.Droplet
	privateKey *rsa.PrivateKey
	conn       *sshutil.Conn
	sshKey     *godo.Key
	uuid       string
}

func (h *Host) Create(w io.Writer) (*host.Info, error) {
	steps := []context.Step{
		{"Creating key", createSSHKey},
		{"Creating host", createDroplet},
		{"Waiting for the host to be ready", waitForSSH},
		{"Creating settings", createEtcFreighterd},
		{"Writing data", writeFiles},
		{"Fetching updates", updateSoftware},
		{"Applying settings", restartFreighterd},
	}
	ctx := context.NewContext(h, steps, w)
	if err := ctx.Execute(); err != nil {
		h.Logf("ERROR: Failed in create host step: %+v", ctx.Error())
		if h.droplet != nil {
			h.Logf("Leaving droplet for inspection: %d", h.droplet.ID)
		}
		return nil, err
	}

	if h.sshKey != nil {
		h.progressLog(w, "Removing key...")
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

func (h *Host) dropletToInfo() *host.Info {
	return &host.Info{
		Provider:       "digitalocean",
		ID:             strconv.Itoa(h.droplet.ID),
		IPAddress:      h.ipAddress(),
		FreighterdPort: uint16(65333),
	}
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
	_, err = WaitForAction(h.Client.Actions, action.ID, StateWaitTime, 15*time.Minute)
	if err != nil {
		return nil, err
	}

	return h.Info()
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
	h.Logf(format, v...)
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
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: defaultConfig["token"]},
	}
	client := godo.NewClient(t.Client())
	host := &Host{
		Config: hostConfig,
		Client: client,
		uuid:   defaultConfig["uuid"],
	}
	return host, nil
}

func init() {
	host.Hosts["digitalocean"] = NewHost
}

func createSSHKey(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	h.Logf("generating temporary SSH key")
	privateKey, err := sshutil.GenerateSSHKey()
	if err != nil {
		return err
	}
	h.Logf("generating temporary SSH key... Done")
	h.privateKey = privateKey
	pubKey, err := sshutil.GetPubKey(privateKey)
	if err != nil {
		return err
	}
	h.Logf("creating SSH keypair on digitalocean")
	sshKey, _, err := h.Client.Keys.Create(&godo.KeyCreateRequest{
		Name:      h.Config.Name,
		PublicKey: string(pubKey),
	})
	if err != nil {
		return err
	}
	h.Logf("creating SSH keypair on digitalocean... Done")
	h.sshKey = sshKey
	return nil
}

func createDroplet(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	config := h.Config
	sshKey := h.sshKey

	h.Logf("digitalocean: creating droplet in region %s, size: %s, slug: %s", config.RegionID, config.SizeID, config.Name)
	normalizedRegion, ok := regionsMap[config.RegionID]
	if !ok {
		return fmt.Errorf("Invalid region or lookup failure: '%s'", config.RegionID)
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
		return err
	}
	h.droplet = dropletRoot.Droplet

	if len(dropletRoot.Links.Actions) > 0 {
		la := dropletRoot.Links.Actions[0]
		if _, err := WaitForAction(h.Client.Actions, la.ID, StateWaitTime, 15*time.Minute); err != nil {
			return err
		}
	} else {
		h.Logf("[WARNING] digitalocean did not return action data for host %+v", hst)
		return nil
	}

	dropletRoot, _, err = h.Client.Droplets.Get(dropletRoot.Droplet.ID)
	if err != nil {
		return fmt.Errorf("create: failed to get droplet after it was successfully created %s", err)
	}

	h.droplet = dropletRoot.Droplet
	return nil
}

func waitForSSH(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	h.Logf("Waiting for SSH to be ready")
	address := fmt.Sprintf("%s:%s", h.ipAddress(), "65100")
	sshConn, err := sshutil.WaitForSSH(address, "root", h.privateKey,
		sshutil.TimeWait, make(chan struct{}))
	if err != nil {
		return err
	}
	h.Logf("SSH READY for droplet %v", h.droplet.ID)
	h.conn = sshConn
	return nil
}

func writeFiles(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	pubKeyR := bytes.NewBuffer(nil)
	for _, k := range h.Config.PublicKeys {
		fmt.Fprintln(pubKeyR, k)
	}
	sourceData := bytes.NewBuffer(nil)
	if err := host.SourceTemplate.Execute(sourceData, h.Config); err != nil {
		return err
	}

	return host.WriteFile(h, h.conn, map[string]io.Reader{
		"/root/.ssh/authorized_keys": pubKeyR,
		host.TLSCALocation:           bytes.NewBufferString(h.Config.TLS.CA),
		host.TLSCertLocation:         bytes.NewBufferString(h.Config.TLS.Cert),
		host.TLSKeyLocation:          bytes.NewBufferString(h.Config.TLS.Key),
		host.SecretLocation:          bytes.NewBufferString(h.Config.Secret),
		host.SourceLocation:          sourceData,
	})
}

func createEtcFreighterd(ctx *context.Context, hst host.Host) error {
	return hst.(*Host).conn.Execute("mkdir -p /etc/freighterd")
}

func updateSoftware(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	return host.UpdateSoftware(h, h.conn)
}

func restartFreighterd(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	return host.RestartFreighterd(h, h.conn)
}

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
