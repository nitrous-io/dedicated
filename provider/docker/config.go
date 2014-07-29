package docker

import (
	"fmt"
	"strings"

	dockerclient "github.com/fsouza/go-dockerclient"
)

var (
	ErrInvalidPortSpec = fmt.Errorf("port spec invalid")
)

type ContainerConfig struct {
	Env          []string `json:"Env"`
	Mounts       []string `json:"Mounts"`
	PortBindings []string `json:"PortBindings"`
}

func NewContainerConfig() *ContainerConfig {
	return &ContainerConfig{}
}

// CreateConfigs returns the dockerclient Config and HostConfig needed to
// create a container.  These can be customized by cfg ContainerConfig.
func (p *DockerProvider) CreateConfigs(containerName, imageName string, cfg *ContainerConfig) (*dockerclient.Config, *dockerclient.HostConfig, error) {
	dcConfig := &dockerclient.Config{
		Hostname:     containerName,
		Image:        imageName,
		Volumes:      map[string]struct{}{},
		ExposedPorts: map[dockerclient.Port]struct{}{},
		Env:          []string{},
	}
	dcHostConfig := &dockerclient.HostConfig{
		Binds:           []string{},
		PortBindings:    map[dockerclient.Port][]dockerclient.PortBinding{},
		PublishAllPorts: true,
	}
	if cfg != nil {
		dcConfig.Env = cfg.Env
		bs, err := dockerPortBindings(cfg.PortBindings)
		if err != nil {
			return nil, nil, err
		}
		dcHostConfig.PortBindings = bs
		for port, _ := range bs {
			dcConfig.ExposedPorts[dockerclient.Port(port)] = struct{}{}
		}
	}

	dcConfig.ExposedPorts[dockerclient.Port("22/tcp")] = struct{}{}
	dcConfig.Volumes["/etc/ssh/freighter"] = struct{}{}
	dcHostConfig.Binds = append(dcHostConfig.Binds, fmt.Sprintf("%s/ssh:/etc/ssh/freighter:ro", p.containerSettingsDir(containerName)))

	return dcConfig, dcHostConfig, nil
}

// StartConfig returns the dockerclient HostConfig needed to start a container.
// It can be customized by cfg ContainerConfig.
func (p *DockerProvider) StartConfig(container *dockerclient.Container, cfg *ContainerConfig) (*dockerclient.HostConfig, error) {
	dcHostConfig := container.HostConfig
	if cfg == nil {
		return dcHostConfig, nil
	}
	if len(cfg.PortBindings) != 0 { // if empty, do not override existing
		bs, err := dockerPortBindings(cfg.PortBindings)
		if err != nil {
			return nil, err
		}
		dcHostConfig.PortBindings = bs
	}
	return dcHostConfig, nil
}

func dockerPortBindings(portSpecs []string) (map[dockerclient.Port][]dockerclient.PortBinding, error) {
	bindings := map[dockerclient.Port][]dockerclient.PortBinding{}
	if len(portSpecs) == 0 {
		return bindings, nil
	}
	for _, portSpec := range portSpecs {
		p, pb, err := parsePortSpec(portSpec)
		if err != nil {
			return nil, err
		}
		if _, ok := bindings[p]; ok {
			bindings[p] = append(bindings[p], pb)
		} else {
			bindings[p] = []dockerclient.PortBinding{pb}
		}
	}
	return bindings, nil
}

// parsePortspec supports any of:
//   ip:hostPort:containerPort
//   ip::containerPort
//   hostPort:containerPort ... :
func parsePortSpec(portSpec string) (dockerclient.Port, dockerclient.PortBinding, error) {
	parts := strings.Split(portSpec, ":")
	var hostIP, hostPort, containerPort string
	switch { // TODO more stringent checking
	case len(parts) == 3 && parts[1] == "": // ip::containerPort
		hostIP, hostPort, containerPort = parts[0], "", parts[2]
	case len(parts) == 3 && parts[1] != "": // ip:hostPort:containerPort
		hostIP, hostPort, containerPort = parts[0], parts[1], parts[2]
	case len(parts) == 2: // hostPort:containerPort
		hostIP, hostPort, containerPort = "127.0.0.1", parts[0], parts[1]
	default:
		return "", dockerclient.PortBinding{}, ErrInvalidPortSpec
	}
	return dockerclient.Port(containerPort),
		dockerclient.PortBinding{HostIp: hostIP, HostPort: hostPort}, nil
}
