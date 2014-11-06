package gce

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
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
	StateWaitTime = 3 * time.Second

	// default timeout when waiting for state
	StateTimeout = 15 * time.Minute

	defaultNetwork = "default" // default network type for GCE instance
	defaultSSHUser = "root"    // default SSH username

	// default to 10GB, but should be set by client
	defaultDiskSize int64 = 10
)

const DriverScopes string = "https://www.googleapis.com/auth/compute " +
	"https://www.googleapis.com/auth/devstorage.full_control"

type HostConfig struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Image string `json:"image_id"`

	// gce specific settings. This document how an instance behaves ( machine
	// type / memory / disk space / custom metdata etc... )
	RegionID    string            `json:"region_id"`
	DiskSizeRaw string            `json:"disk_size"`
	MachineType string            `json:"size_id"`
	Metadata    map[string]string `json:"metadata"`
	DiskSize    int64

	PublicKeys []string        `json:"public_keys"`
	TLS        *host.TLSConfig `json:"tls"`
	Secret     string          `json:"secret"`

	// for setting up repository on the host
	AptUsername string `json:"apt_username"` // username of nitrous.io
	AptPassword string `json:"apt_password"` // nitrous.io access token
	AptLocation string `json:"apt_location"` // URI of the repository
	AptDist     string `json:"apt_dist"`     // distribution of the repo
}

type Host struct {
	Config    *HostConfig
	service   *compute.Service
	projectID string

	uuid       string
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
		{"Writing data", writeFiles},
		{"Fetching updates", updateSoftware},
		{"Applying settings", restartFreighterd},
	}
	ctx := context.NewContext(h, steps, w)
	if err := ctx.Execute(); err != nil {
		if h.instance != nil {
			h.progressLog(w, "Canceling host creation...")
			zone := h.Config.RegionID
			resp, derr := h.service.Instances.Delete(h.projectID, zone, h.instance.Name).Do()
			if derr != nil {
				h.progressLog(w, "Canceling host creation...Failed!")
				h.Logf("ERROR: Failed to remove instance zone=%s instance=%s", zone, h.instance.Name)
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
	zone := h.Config.RegionID
	name := h.Config.ID
	instance, err := h.service.Instances.Get(h.projectID, zone, name).Do()
	if err != nil {
		// if the instance does not exist, then consider that it is sucessfully
		// destroyed
		e, ok := err.(*googleapi.Error)
		if ok && e.Code == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("gce: failed to get zone=%s name=%s, %q", zone, name, err)
	}
	h.instance = instance

	_, err = h.service.Instances.Delete(h.projectID, zone, name).Do()
	if err != nil {
		// if the instance does not exist, then consider that it is sucessfully
		// destroyed
		e, ok := err.(*googleapi.Error)
		if ok && e.Code == http.StatusNotFound {
			return nil
		}
		return fmt.Errorf("failed to delete %s/%s, %q", zone, name)
	}
	return nil
}

func (h *Host) Info() (*host.Info, error) {
	zone := h.Config.RegionID
	name := h.Config.ID
	instance, err := h.service.Instances.Get(h.projectID, zone, name).Do()
	if err != nil {
		return nil, fmt.Errorf("gce: failed to get zone=%s name=%s, %q", zone, name, err)
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
		RegionID:       h.Config.RegionID,
	}
}

// Restart does a hard-reset of a GCE instance
func (h *Host) Restart() (*host.Info, error) {
	zone := h.Config.RegionID
	name := h.Config.ID
	op, err := h.service.Instances.Reset(h.projectID, zone, name).Do()
	if err != nil {
		return nil, fmt.Errorf("gce: failed to get zone=%s name=%s, %q", zone, name, err)
	}
	instance, err := h.waitForTransition(op, "RUNNING", zone, name)
	if err != nil {
		return nil, err
	}
	h.instance = instance
	return h.instanceInfo(), nil
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
	// if we dont specify a disk size, use a default disk size
	if hostConfig.DiskSizeRaw == "" {
		hostConfig.DiskSize = defaultDiskSize
	} else {
		s, err := strconv.ParseInt(hostConfig.DiskSizeRaw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid disk size %s", hostConfig.DiskSizeRaw)
		}
		hostConfig.DiskSize = s
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

	return &Host{
		uuid:      defaultConfig["uuid"],
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
	zoneName := h.Config.RegionID
	// get zone
	zone, err := h.service.Zones.Get(h.projectID, zoneName).Do()
	if err != nil {
		return fmt.Errorf("failed to get projectid=%s zone=%s, %q",
			h.projectID, zoneName, err)
	}
	// get image
	image, err := h.service.Images.Get(h.projectID, h.Config.Image).Do()
	if err != nil {
		return fmt.Errorf("failed to get projectid=%s image=%s, %q",
			h.projectID, h.Config.Image, err)
	}
	// get machine type
	machineType, err := h.service.MachineTypes.Get(h.projectID, zone.Name, h.Config.MachineType).Do()
	if err != nil {
		return fmt.Errorf("failed to get projectid=%s zone=%s machine=%s, %q",
			h.projectID, zone.Name, h.Config.MachineType, err)
	}
	// get network
	network, err := h.service.Networks.Get(h.projectID, defaultNetwork).Do()
	if err != nil {
		return fmt.Errorf("failed to get projectid=%s zone=%s network=%s, %q",
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

	instance, err := h.waitForTransition(op, "RUNNING", zone.Name, h.instance.Name)
	if err != nil {
		return err
	}
	h.instance = instance
	return nil
}

func waitForSSH(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
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
	h.Logf("gce: SSH READY for instance %q", h.instance.Name)
	h.conn = sshConn
	return nil
}

// get public ip address of the instance. This is usually the first non-empty
// NatIP in Access Config
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

func writeFiles(ctx *context.Context, hst host.Host) error {
	h := hst.(*Host)
	sourceData := bytes.NewBuffer(nil)
	if err := host.SourceTemplate.Execute(sourceData, h.Config); err != nil {
		return err
	}

	return host.WriteFile(h, h.conn, map[string]io.Reader{
		host.TLSCALocation:   bytes.NewBufferString(h.Config.TLS.CA),
		host.TLSCertLocation: bytes.NewBufferString(h.Config.TLS.Cert),
		host.TLSKeyLocation:  bytes.NewBufferString(h.Config.TLS.Key),
		host.SecretLocation:  bytes.NewBufferString(h.Config.Secret),
		host.SourceLocation:  sourceData,
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
	if h.instance != nil {
		parts = append(parts, fmt.Sprintf("zone=%s instance=%s", h.Config.RegionID, h.instance.Name))
	}
	log.Printf(fmt.Sprintf("%s %s", strings.Join(parts, " "), format), v...)
}

// progressLog writes the message to the given writer and logs the message,
// prefixing host information.  It is used for user-facing messages.  Use
// `hlog` for internal logging.
func (h *Host) progressLog(w io.Writer, format string, v ...interface{}) {
	fmt.Fprintf(w, format, v...)
	h.Logf(format, v...)
}
