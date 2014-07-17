package docker

import (
	"encoding/json"
	"fmt"
	"strings"

	dockerclient "github.com/fsouza/go-dockerclient"
)

type ContainerConfig struct {
	Env          []string
	Volumes      []string
	VolumesFrom  []string
	ExposedPorts []string
	PortBindings map[string]string
}

// subset of dockerclient.Config, used as whitelist
type allowedConfig struct {
	Env          []string            `json:"Env,omitempty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
	Volumes      map[string]struct{} `json:"Volumes,omitempty"`
}

// subset of dockerclient.HostConfig, used as whitelist
type allowedHostConfig struct {
	PortBindings map[string][]dockerclient.PortBinding `json:"PortBindings,omitempty"`
	VolumesFrom  []string                              `json:"VolumesFrom,omitempty"`
}

// configParam is used to deserialize JSON from the config parameter in calls
// to create/start container
type configParam struct {
	Config     *allowedConfig     `json:"Config,omitempty"`
	HostConfig *allowedHostConfig `json:"HostConfig,omitempty"`
}

func NewContainerConfig() *ContainerConfig {
	return &ContainerConfig{}
}

func (conf ContainerConfig) MarshalJSON() ([]byte, error) {
	cp := configParam{
		Config: &allowedConfig{}, HostConfig: &allowedHostConfig{},
	}
	if len(conf.Env) > 0 {
		cp.Config.Env = conf.Env
	}
	if len(conf.ExposedPorts) > 0 {
		cp.Config.ExposedPorts = make(map[string]struct{})
		for _, port := range conf.ExposedPorts {
			cp.Config.ExposedPorts[port] = struct{}{}
		}
	}
	if len(conf.PortBindings) > 0 {
		cp.HostConfig.PortBindings = make(map[string][]dockerclient.PortBinding)
		for cPort, hDest := range conf.PortBindings {
			hParts := strings.Split(hDest, ":")
			if len(hParts) != 2 {
				return []byte{}, fmt.Errorf("Error parsing port binding %s: %s", cPort, hDest)
			}
			hostIP := hParts[0]
			// support ':1234', defaults to 127.0.0.1:1234
			if hostIP == "" {
				hostIP = "127.0.0.1"
			}
			hostPort := hParts[1]
			cp.HostConfig.PortBindings[cPort] = []dockerclient.PortBinding{
				{HostIp: hostIP, HostPort: hostPort},
			}
		}
	}
	if len(conf.Volumes) > 0 {
		cp.Config.Volumes = make(map[string]struct{})
		for _, path := range conf.Volumes {
			cp.Config.Volumes[path] = struct{}{}
		}
	}
	cp.HostConfig.VolumesFrom = conf.VolumesFrom
	return json.Marshal(cp)
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
