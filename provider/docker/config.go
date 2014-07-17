package docker

import (
	"encoding/json"
	"strings"

	dockerclient "github.com/fsouza/go-dockerclient"
)

// subset of dockerclient.Config, used as whitelist
type allowedConfig struct {
	Env          []string
	ExposedPorts map[string]struct{}
	Volumes      map[string]struct{}
}

// subset of dockerclient.HostConfig, used as whitelist
type allowedHostConfig struct {
	PortBindings map[string][]dockerclient.PortBinding
	VolumesFrom  []string
}

// configParam is used to deserialize JSON from the config parameter in calls
// to create/start container
type configParam struct {
	Config     *allowedConfig
	HostConfig *allowedHostConfig
}

// parseConfigParam takes json that looks like
// {"Config": { ...container create config... },
//  "HostConfig": { ...container start config }}
// and returns (config json, host config json)
func parseConfigParam(configJSON string) (*allowedConfig, *allowedHostConfig, error) {
	if configJSON == "" {
		configJSON = "{}"
	}
	j := configParam{}
	err := json.NewDecoder(strings.NewReader(configJSON)).Decode(&j)
	if err != nil {
		return nil, nil, err
	}
	return j.Config, j.HostConfig, nil
}

// overrideCreateConfig takes an existing dockerclient config and merges in
// allowedConfig parsed from JSON
// TODO: Docker Remote API does not validate, so we have to do our own
// validation, i.e. env var without '='
func overrideConfig(c *dockerclient.Config, override *allowedConfig) {
	if override == nil {
		return
	}
	c.Env = override.Env
	if override.ExposedPorts != nil {
		for k, v := range override.ExposedPorts {
			c.ExposedPorts[dockerclient.Port(k)] = v
		}
	}
	if override.Volumes != nil {
		for k, v := range override.Volumes {
			c.Volumes[k] = v
		}
	}
}

// overrideHostConfig is similar to overrideCreateConfig, but with
// dockerclient.HostConfig instead of dockerclient.Config
func overrideHostConfig(hc *dockerclient.HostConfig, override *allowedHostConfig) {
	if override == nil {
		return
	}
	if override.PortBindings != nil {
		hc.PortBindings = make(map[dockerclient.Port][]dockerclient.PortBinding)
		for k, v := range override.PortBindings {
			hc.PortBindings[dockerclient.Port(k)] = v
		}
	}
	if override.VolumesFrom != nil {
		hc.VolumesFrom = override.VolumesFrom
	}
}
