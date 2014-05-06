package digitalocean

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	host "github.com/nitrous-io/hostbuilder/host"
	"github.com/nitrous-io/hostbuilder/ssh"
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
	Client *DigitalOceanClient
	Config *HostConfig

	instance *Droplet
}

func (h *Host) Create() error {
	config := h.Config
	// create a tmp keypair
	privateKey, err := ssh.GenerateSSHKey()
	if err != nil {
		return err
	}
	pubKey, err := ssh.GetPubKey(privateKey)
	if err != nil {
		return err
	}
	sshKey, err := h.Client.CreateKey(h.Config.Name, string(pubKey))
	if err != nil {
		return err
	}
	// remove the tmp keypair when we dont use it anymore
	defer h.Client.DestroyKey(sshKey.Id)

	// create the droplet
	droplet, err := h.Client.CreateDroplet(config.Name, config.SizeId,
		config.ImageId, config.RegionId, 0, false)
	if err != nil {
		return err
	}
	h.instance = droplet

	// wait for the droplet to be active
	if err := waitForState("active", uint(h.instance.Id), h.Client, 5*time.Minute); err != nil {
		return err
	}

	// check that we can ssh into the server
	address := fmt.Sprintf("%s:%s", h.instance.IpAddress, "22")
	if _, err := ssh.WaitForSSH(address, "root", privateKey, make(chan struct{})); err != nil {
		return err
	}
	return nil
}

func (h *Host) Destroy() error {
	id, err := strconv.ParseUint(h.Config.Id, 10, 64)
	if err != nil {
		return err
	}
	return h.Client.DestroyDroplet(uint(id))
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	client := NewDigitalOceanClient(defaultConfig["client_id"],
		defaultConfig["api_key"])
	hostConfig := &HostConfig{}
	if err := json.Unmarshal([]byte(config), hostConfig); err != nil {
		log.Printf("digitalocean: invalid config %s", config)
		return nil, err
	}
	host := &Host{
		Config: hostConfig,
		Client: client,
	}
	return host, nil
}

func init() {
	host.Hosts["digitalocean"] = NewHost
}
