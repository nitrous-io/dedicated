package digitalocean

import (
	"encoding/json"
	host "github.com/nitrous-io/hostbuilder/host"
	"log"
	"strconv"
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
	droplet, err := h.Client.CreateDroplet(config.Name, config.SizeId,
		config.ImageId, config.RegionId, 0, false)
	if err != nil {
		return err
	}
	h.instance = droplet
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
