package gce

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"crypto/rsa"

	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/goauth2/oauth/jwt"
	compute "code.google.com/p/google-api-go-client/compute/v1"
	"code.google.com/p/google-api-go-client/googleapi"
	"github.com/nitrous-io/dedicated/hostbuilder/context"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/pkg/sshutil"
)

var (
	TLSCALocation   = "/etc/freighterd/ca.crt"
	TLSCertLocation = "/etc/freighterd/cert.crt"
	TLSKeyLocation  = "/etc/freighterd/key.crt"
	SecretLocation  = "/etc/freighterd/secret"

	StateWaitTime = 3 * time.Second

	// default timeout when waiting for state
	StateTimeout = 15 * time.Minute

	// FIXME: the map should belongs to client, not the builder
	regionsMap = map[string]string{
		"sgp": "sgp1",
		"nyc": "nyc2",
		"sfo": "sfo1",
		"ams": "ams2",
		"lon": "lon1",
	}

	defaultNetwork = "default" // default network type for GCE instance
	defaultSSHUser = "root"    // default SSH username

	// default to 10GB, but should be set by client
	defaultDiskSize int64 = 10
)

type tlsConfig struct {
	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

const DriverScopes string = "https://www.googleapis.com/auth/compute " +
	"https://www.googleapis.com/auth/devstorage.full_control"

type HostConfig struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Zone        string            `json:"zone"`
	DiskSize    int64             `json:"disk_size"`
	MachineType string            `json:"machine_type"`
	PublicKeys  []string          `json:"public_keys"`
	TLS         *tlsConfig        `json:"tls"`
	Secret      string            `json:"secret"`
	Metadata    map[string]string `json:"metadata"`
}

type Host struct {
	Config    *HostConfig
	service   *compute.Service
	projectID string

	conn       *sshutil.Conn
	privateKey *rsa.PrivateKey
	publicKey  []byte // tmp public key for communicating with the host
	instance   *compute.Instance
}

func (h *Host) Create(w io.Writer) (*host.Info, error) {
	steps := []context.Step{
		{"Creating key", createSSHKey},
		{"Creating host", createInstance},
		{"Waiting for the host to be ready", waitForSSH},
		{"Creating settings", createEtcFreighterd},
		{"Writing certificates", writeTLSCerts},
		{"Writing token", writeSecret},
		{"Setting up repos", writeRepository},
		{"Fetching updates", updateSoftware},
	}
	ctx := context.NewContext(h, steps, w)
	if err := ctx.Execute(); err != nil {
		if h.instance != nil {
			h.progressLog(w, "Canceling host creation...")
			resp, derr := h.service.Instances.Delete(h.projectID, h.instance.Zone, h.instance.Name).Do()
			if derr != nil {
				h.progressLog(w, "Canceling host creation...Failed!")
				hlog(h, "ERROR: Failed to remove droplet %s/%s", h.instance.Zone, h.instance.Name)
				return nil, fmt.Errorf("Failed to destroy after create error: %s, destroy error: %v, resp: %v", err, derr, resp)
			}
			h.progressLog(w, "Canceling host creation...Done!")
		}
		return nil, err
	}

	info := h.instanceInfo()
	h.progressLog(w, "Provisioning host...Done!")
	return info, nil
}

func (h *Host) Destroy() error {
	instance, err := h.service.Instances.Get(h.projectID, h.Config.Zone, h.Config.ID).Do()
	if err != nil {
		// if the instance does not exist, then consider that it is sucessfully
		// destroyed
		e, ok := err.(*googleapi.Error)
		if ok && e.Code == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("failed to get GCE %s/%s, %q", h.Config.Zone,
			h.Config.ID, err)
	}
	h.instance = instance

	_, err = h.service.Instances.Delete(h.projectID, h.instance.Zone, h.instance.Name).Do()
	if err != nil {
		// if the instance does not exist, then consider that it is sucessfully
		// destroyed
		e, ok := err.(*googleapi.Error)
		if ok && e.Code == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("failed to delete %s/%s, %q", h.instance.Zone, h.instance.Name)
	}
	return nil
}

func (h *Host) Info() (*host.Info, error) {
	instance, err := h.service.Instances.Get(h.projectID, h.Config.Zone, h.Config.ID).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get GCE %s/%s, %q", h.Config.Zone,
			h.Config.ID, err)
	}
	h.instance = instance
	return h.instanceInfo(), nil
}

func (h *Host) instanceInfo() *host.Info {
	return &host.Info{
		Provider:       "gce",
		ID:             h.instance.Name,
		IPAddress:      h.ipAddress(),
		FreighterdPort: uint16(65333),
	}
}

func (h *Host) Restart() (*host.Info, error) {
	return nil, fmt.Errorf("not implemented")
}

// defaultConfig structure
// - auth_uri
// - token_uri
// - private_key_id
// - private_key
// - client_email
// - client_id
func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		log.Printf("gce: invalid config %s", config)
		return nil, fmt.Errorf("invalid config %q", err)
	}
	if hostConfig.DiskSize == 0 {
		hostConfig.DiskSize = defaultDiskSize
	}

	log.Printf("requesting GCE token with email %q", defaultConfig["client_email"])
	t := jwt.NewToken(
		defaultConfig["client_email"],
		DriverScopes,
		[]byte(defaultConfig["private_key"]),
	)
	t.ClaimSet.Aud = defaultConfig["token_uri"]
	c := &http.Client{}
	token, err := t.Assert(c)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCE token %q", err)
	}

	// Instantiate the transport
	transport := &oauth.Transport{
		Config: &oauth.Config{
			ClientId: defaultConfig["client_id"],
			Scope:    DriverScopes,
			TokenURL: defaultConfig["token_uri"],
			AuthURL:  defaultConfig["auth_uri"],
		},
		Token: token,
	}

	// initialize GCE service
	service, err := compute.New(transport.Client())
	if err != nil {
		return nil, fmt.Errorf("failed to create GCE service %q", err)
	}

	log.Printf("config %#v", hostConfig)
	return &Host{
		projectID: defaultConfig["project_id"],
		service:   service,
		Config:    hostConfig,
	}, nil
}

func init() {
	host.Hosts["gce"] = NewHost
}

func createSSHKey(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	// create a tmp keypair
	privateKey, err := sshutil.GenerateSSHKey()
	if err != nil {
		return err
	}
	h.privateKey = privateKey
	pubKey, err := sshutil.GetPubKey(privateKey)
	if err != nil {
		return err
	}
	h.publicKey = pubKey
	return nil
}

func createInstance(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	// get zone
	zone, err := h.service.Zones.Get(h.projectID, h.Config.Zone).Do()
	if err != nil {
		return fmt.Errorf("failed to get zone %s/zones/%s, %q",
			h.projectID, h.Config.Zone, err)
	}
	// get image
	image, err := h.service.Images.Get(h.projectID, h.Config.Image).Do()
	if err != nil {
		return fmt.Errorf("failed to get image %s/images/%s, %q",
			h.projectID, h.Config.Image, err)
	}
	// get machine type
	machineType, err := h.service.MachineTypes.Get(h.projectID, zone.Name, h.Config.MachineType).Do()
	if err != nil {
		return fmt.Errorf("failed to get machine type %s/%s/%s, %q",
			h.projectID, zone.Name, h.Config.MachineType, err)
	}
	// get network
	network, err := h.service.Networks.Get(h.projectID, defaultNetwork).Do()
	if err != nil {
		return fmt.Errorf("failed to get network %s/%s/%s, %q",
			h.projectID, zone.Name, defaultNetwork, err)
	}
	// get medadata
	metadata := h.metadata(append(h.Config.PublicKeys, string(h.publicKey)))

	// Create the instance information
	h.instance = &compute.Instance{
		Name: h.Config.Name,
		Disks: []*compute.AttachedDisk{
			&compute.AttachedDisk{
				Type:       "PERSISTENT",
				Mode:       "READ_WRITE",
				Kind:       "compute#attachedDisk",
				Boot:       true,
				AutoDelete: true,
				InitializeParams: &compute.AttachedDiskInitializeParams{
					SourceImage: image.SelfLink,
					DiskSizeGb:  h.Config.DiskSize,
				},
			},
		},
		MachineType: machineType.SelfLink,
		Metadata: &compute.Metadata{
			Items: metadata,
		},
		NetworkInterfaces: []*compute.NetworkInterface{
			&compute.NetworkInterface{
				AccessConfigs: []*compute.AccessConfig{
					&compute.AccessConfig{
						Name: "AccessConfig",
						Type: "ONE_TO_ONE_NAT",
					},
				},
				Network: network.SelfLink,
			},
		},
	}

	op, err := h.service.Instances.Insert(h.projectID, zone.Name, h.instance).Do()
	if err != nil {
		return err
	}

	var errCh chan error

	// wait for the create operation to be done
	errCh = make(chan error, 1)
	go waitForState(errCh, "DONE", h.refreshOp(zone.Name, op))
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case <-time.After(StateTimeout):
		return fmt.Errorf("timeout while waiting for instance %q to be created", h.instance.Name)
	}

	// wait for instance state to be running
	errCh = make(chan error, 1)
	go waitForState(errCh, "RUNNING", h.refreshInstanceState(zone.Name, h.instance.Name))
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case <-time.After(StateTimeout):
		return fmt.Errorf("timeout while waiting for instance %q to be running", h.instance.Name)
	}

	// refetch instance
	instance, err := h.service.Instances.Get(h.projectID, zone.Name, h.instance.Name).Do()
	if err != nil {
		return err
	}
	h.instance = instance
	return nil
}

func waitForSSH(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	// check that we can ssh into the server. use 22 for now
	ip := h.ipAddress()
	if ip == "" {
		return fmt.Errorf("invalid ip, instance %#v %#v", h.instance, h.instance.NetworkInterfaces[0])
	}
	address := fmt.Sprintf("%s:%s", h.ipAddress(), "65100")
	sshConn, err := sshutil.WaitForSSH(address, "root", h.privateKey,
		sshutil.TimeWait, make(chan struct{}))
	if err != nil {
		return err
	}
	log.Printf("gce: SSH READY for instance %q", h.instance.Name)
	h.conn = sshConn
	return nil
}

func (h *Host) ipAddress() string {
	for _, ni := range h.instance.NetworkInterfaces {
		if ni.AccessConfigs == nil {
			continue
		}

		for _, ac := range ni.AccessConfigs {
			if ac.NatIP != "" {
				return ac.NatIP
			}
		}
	}
	return ""
}

// set metadata of the host. This replaces SSH keys if we pass them
func (h *Host) metadata(sshKeys []string) []*compute.MetadataItems {
	m := make(map[string]string)

	// Copy metadata from config
	for k, v := range h.Config.Metadata {
		m[k] = v
	}

	if len(sshKeys) > 0 {
		sshConf := ""
		for _, k := range sshKeys {
			sshConf += fmt.Sprintf("%s:%s\n", defaultSSHUser, k)
		}
		m["sshKeys"] = sshConf
	}

	metadata := make([]*compute.MetadataItems, len(m))
	for k, v := range m {
		metadata = append(metadata, &compute.MetadataItems{Key: k, Value: v})
	}
	return metadata
}

// stateRefreshFunc is used to refresh the state of a thing and is
// used in conjunction with waitForState.
type stateRefreshFunc func() (string, error)

func (h *Host) refreshOp(zone string, op *compute.Operation) stateRefreshFunc {
	return func() (string, error) {
		newOp, err := h.service.ZoneOperations.Get(h.projectID, zone, op.Name).Do()
		if err != nil {
			return "", err
		}

		// If the op is done, check for errors
		err = nil
		if newOp.Status == "DONE" {
			if newOp.Error != nil {
				var errMsgs []string
				for _, e := range newOp.Error.Errors {
					errMsgs = append(errMsgs, e.Message)
				}

				err = errors.New(strings.Join(errMsgs, "\n"))
			}
		}

		return newOp.Status, err
	}
}

func (h *Host) WaitForInstance(state, zone, name string) error {
	errCh := make(chan error, 1)
	go waitForState(errCh, state, h.refreshInstanceState(zone, name))
	select {
	case err := <-errCh:
		return err
	case <-time.After(StateTimeout):
		return fmt.Errorf("timeout while waiting for instance %q to be running", h.instance.Name)
	}
}

func (h *Host) refreshInstanceState(zone, name string) stateRefreshFunc {
	return func() (string, error) {
		instance, err := h.service.Instances.Get(h.projectID, zone, name).Do()
		if err != nil {
			return "", err
		}
		return instance.Status, nil
	}
}

// waitForState will spin in a loop forever waiting for state to
// reach a certain target.
func waitForState(errCh chan<- error, target string, refresh stateRefreshFunc) {
	for {
		state, err := refresh()
		if err != nil {
			errCh <- err
			return
		}
		if state == target {
			errCh <- nil
			return
		}

		time.Sleep(StateWaitTime)
	}
}

func createEtcFreighterd(ctx *context.Context, hst host.Host) error {
	return hst.(*Host).conn.Execute("mkdir -p /etc/freighterd")
}

func writeTLSCerts(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	return writeFile(h, map[string]io.Reader{
		TLSCALocation:   bytes.NewBufferString(h.Config.TLS.CA),
		TLSCertLocation: bytes.NewBufferString(h.Config.TLS.Cert),
		TLSKeyLocation:  bytes.NewBufferString(h.Config.TLS.Key),
	})
}

func writeSecret(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	return writeFile(h, map[string]io.Reader{
		SecretLocation: bytes.NewBufferString(h.Config.Secret),
	})
}

func writeFile(h *Host, files map[string]io.Reader) error {
	flist := []string{}
	for dest, val := range files {
		if err := h.conn.Upload(dest, val); err != nil {
			return err
		}
		flist = append(flist, dest)
	}
	log.Printf("Uploaded (%s) to %v", strings.Join(flist, " "), h.ipAddress())
	return nil
}

func writeRepository(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	sourceData := bytes.NewBuffer(nil)
	if err := host.SourceTemplate.Execute(sourceData, h.Config); err != nil {
		return err
	}
	if err := h.conn.Upload(host.SourceLocation, sourceData); err != nil {
		return err
	}
	log.Printf("Uploaded apt details at %v", h.ipAddress())
	return nil
}

func updateSoftware(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	updateSoftwareCmd :=
		`apt-get update && apt-get install -yq freighterd && service freighterd restart`
	if err := h.conn.Execute(updateSoftwareCmd); err != nil {
		return err
	}
	log.Printf("Updated software on %v", h.ipAddress())
	return nil
}

func hlog(h *Host, format string, v ...interface{}) {
	parts := []string{}
	//if h.uuid != "" {
	//    parts = append(parts, fmt.Sprintf("uuid=%s", h.uuid))
	//}
	if h.instance != nil {
		parts = append(parts, fmt.Sprintf("zone=%s instance=%s", h.instance.Zone, h.instance.Name))
	}
	log.Printf(fmt.Sprintf("%s %s", strings.Join(parts, " "), format), v...)
}

// progressLog writes the message to the given writer and logs the message,
// prefixing host information.  It is used for user-facing messages.  Use
// `hlog` for internal logging.
func (h *Host) progressLog(w io.Writer, format string, v ...interface{}) {
	fmt.Fprintf(w, format, v...)
	hlog(h, format, v...)
}
