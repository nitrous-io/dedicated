package hostbuilder

import (
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	_ "github.com/nitrous-io/dedicated/hostbuilder/host/amazon"
	_ "github.com/nitrous-io/dedicated/hostbuilder/host/digitalocean"
	_ "github.com/nitrous-io/dedicated/hostbuilder/host/docker"
	_ "github.com/nitrous-io/dedicated/hostbuilder/host/gce"
)

func InitHost(provider string, defaultConfig map[string]string, config string) (host.Host, error) {
	return host.InitHost(provider, defaultConfig, config)
}
