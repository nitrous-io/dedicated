package docker

import (
	dockerclient "github.com/fsouza/go-dockerclient"

	"github.com/nitrous-io/freighter/provider"
)

func (p *DockerProvider) CreateConfigs(containerName, imageName string, cfg *provider.ContainerConfig) (*dockerclient.Config, *dockerclient.HostConfig, error) {
	return p.createConfigs(containerName, imageName, cfg)
}

func (p *DockerProvider) StartConfig(container *dockerclient.Container, cfg *provider.ContainerConfig) (*dockerclient.HostConfig, error) {
	return p.startConfig(container, cfg)
}
