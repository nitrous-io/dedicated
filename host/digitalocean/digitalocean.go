package digitalocean

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	host "github.com/nitrous-io/hostbuilder/host"
	do "github.com/nitrous-io/hostbuilder/pkg/digitalocean"
	"github.com/nitrous-io/hostbuilder/pkg/ssh"
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
	Client *do.Client
	Config *HostConfig

	instance *do.Droplet
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
		config.ImageId, config.RegionId, sshKey.Id, false)
	if err != nil {
		return err
	}

	// wait for the droplet to be active
	droplet, err = do.WaitForState("active", droplet.Id, h.Client,
		do.StateWaitTime, 5*time.Minute)
	if err != nil {
		return err
	}
	h.instance = droplet

	// check that we can ssh into the server
	address := fmt.Sprintf("%s:%s", h.instance.IpAddress, "22")
	if _, err := ssh.WaitForSSH(address, "root", privateKey,
		ssh.TimeWait, make(chan struct{})); err != nil {
		return err
	}
	log.Println("SSH READY for droplet %v", h.instance.Id)
	return nil
}

func (h *Host) Destroy() error {
	id, err := strconv.ParseInt(h.Config.Id, 10, 64)
	if err != nil {
		return err
	}
	return h.Client.DestroyDroplet(int(id))
}

func NewHost(defaultConfig map[string]string, config string) (host.Host, error) {
	client := do.NewClient(defaultConfig["client_id"], defaultConfig["api_key"])
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
