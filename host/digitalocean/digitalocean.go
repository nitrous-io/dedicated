package digitalocean

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/mitchellh/goamz/route53"
	"github.com/nitrous-io/hostbuilder/host"
	"github.com/nitrous-io/hostbuilder/host/amazon"
	do "github.com/nitrous-io/hostbuilder/pkg/digitalocean"
	"github.com/nitrous-io/hostbuilder/pkg/ssh"
)

const (
	DNSCreate = "CREATE"
	DNSDelete = "DELETE"
)

var (
	MissingZoneIdError     = errors.New("missing zone id")
	MissingBaseDomainError = errors.New("missing base domain")
)

type HostConfig struct {
	Id         string   `json:"id"`
	Name       string   `json:"name"`
	ImageId    string   `json:"image_id"`
	RegionId   string   `json:"region_id"`
	SizeId     string   `json:"size_id"`
	PublicKeys []string `json:"public_keys"`
}

type Host struct {
	Client     *do.Client
	Config     *HostConfig
	BaseDomain string
	ZoneId     string

	instance *do.Droplet
	router   *route53.Route53
}

func (h *Host) dnsRecord(dnsType string) *route53.ChangeResourceRecordSetsRequest {
	domain := h.Config.Name + "." + h.BaseDomain
	return &route53.ChangeResourceRecordSetsRequest{
		Action: dnsType,
		Name:   domain,
		Type:   "A",
		Value:  h.instance.IpAddress,
	}
}

func (h *Host) createDNSRecord() error {
	record := h.dnsRecord(DNSCreate)
	_, err := h.router.ChangeResourceRecordSet(record, h.ZoneId)
	return err
}

func (h *Host) deleteDNSRecord() error {
	record := h.dnsRecord(DNSDelete)
	_, err := h.router.ChangeResourceRecordSet(record, h.ZoneId)
	return err
}

func (h *Host) Create() (*host.Info, error) {
	config := h.Config
	// create a tmp keypair
	privateKey, err := ssh.GenerateSSHKey()
	if err != nil {
		return nil, err
	}
	pubKey, err := ssh.GetPubKey(privateKey)
	if err != nil {
		return nil, err
	}
	sshKey, err := h.Client.CreateKey(h.Config.Name, string(pubKey))
	if err != nil {
		return nil, err
	}
	// remove the tmp keypair when we dont use it anymore
	defer h.Client.DestroyKey(sshKey.Id)

	// create the droplet
	droplet, err := h.Client.CreateDroplet(config.Name, config.SizeId,
		config.ImageId, config.RegionId, sshKey.Id, false)
	if err != nil {
		return nil, err
	}

	// wait for the droplet to be active
	droplet, err = do.WaitForState("active", droplet.Id, h.Client,
		do.StateWaitTime, 5*time.Minute)
	if err != nil {
		return nil, err
	}
	h.instance = droplet

	// check that we can ssh into the server
	address := fmt.Sprintf("%s:%s", h.instance.IpAddress, "22")
	sshConn, err := ssh.WaitForSSH(address, "root", privateKey,
		ssh.TimeWait, make(chan struct{}))
	if err != nil {
		return nil, err
	}
	log.Println("SSH READY for droplet %v", h.instance.Id)

	// write the public keys into server
	// This needs /root/.ssn/authorized_keys to be accessible
	pubKeyR := bytes.NewBufferString("")
	for _, k := range h.Config.PublicKeys {
		fmt.Fprintln(pubKeyR, k)
	}
	if err := sshConn.Upload("/root/.ssh/authorized_keys", pubKeyR); err != nil {
		return nil, err
	}
	log.Printf("SSH ready at %v", h.instance.IpAddress)

	// setup route53 domain
	if err := h.createDNSRecord(); err != nil {
		return nil, err
	}
	log.Printf("DNS record created for %v", h.instance.IpAddress)
	return h.Info()
}

func (h *Host) Destroy() error {
	id64, err := strconv.ParseInt(h.Config.Id, 10, 64)
	if err != nil {
		return err
	}
	id := int(id64)
	droplet, err := h.Client.DropletInfo(id)
	if err != nil {
		return err
	}
	h.instance = droplet
	if err := h.deleteDNSRecord(); err != nil {
		return err
	}
	if err := h.Client.DestroyDroplet(id); err != nil {
		return err
	}
	return nil
}

func (h *Host) Info() (*host.Info, error) {
	if h.instance == nil {
		return nil, errors.New("Invalid ec2 instance")
	}
	info := &host.Info{
		Provider:  "digitalocean",
		Id:        strconv.Itoa(h.instance.Id),
		IpAddress: h.instance.IpAddress,
	}
	return info, nil
}

func (h *Host) Stop() error {
	panic("do provider does not support stop yet")
}

func (h *Host) Start() error {
	panic("do provider does not support start yet")
}

func (h *Host) Restart() error {
	panic("do provider does not support restart yet")
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	client := do.NewClient(defaultConfig["client_id"], defaultConfig["api_key"])
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		log.Printf("digitalocean: invalid config %s", config)
		return nil, err
	}
	accessConfig := &amazon.AccessConfig{
		AccessKey: defaultConfig["aws_access_key"],
		SecretKey: defaultConfig["aws_secret_key"],
	}
	auth, err := accessConfig.Auth()
	if err != nil {
		log.Printf("amazon: auth failed: %v", err)
		return nil, err
	}
	if len(defaultConfig["zone_id"]) == 0 {
		return nil, MissingZoneIdError
	}
	if len(defaultConfig["base_domain"]) == 0 {
		return nil, MissingBaseDomainError
	}
	host := &Host{
		Config:     hostConfig,
		Client:     client,
		ZoneId:     defaultConfig["zone_id"],
		BaseDomain: defaultConfig["base_domain"],
		router:     route53.NewRoute53(auth),
	}
	return host, nil
}

func init() {
	host.Hosts["digitalocean"] = NewHost
}
