package docker

import (
	"errors"

	dockerclient "github.com/fsouza/go-dockerclient"

	"github.com/nitrous-io/freighter/provider"
	"github.com/nitrous-io/freighter/util"
)

func (p *DockerProvider) CreateConfigs(containerName, imageName string, cfg *provider.ContainerConfig) (*dockerclient.Config, *dockerclient.HostConfig, error) {
	return p.createConfigs(containerName, imageName, cfg)
}

func (p *DockerProvider) StartConfig(container *dockerclient.Container, cfg *provider.ContainerConfig) (*dockerclient.HostConfig, error) {
	return p.startConfig(container, cfg)
}

// Client exposes private client in tests
func (p *DockerProvider) Client() *dockerclient.Client {
	return p.client
}

// Specify another docker host instead of the default unix socket
// This is useful when we want to inject a fake docker host or when we want to
// use FakeRoundtripper with a dummy tcp address
func NewDockerProviderWithHost(logger *util.Logger, dataPath string, host string) (*DockerProvider, error) {
	if logger == nil || dataPath == "" {
		return nil, errors.New("docker.NewDockerProvider: invalid argument")
	}
	client, err := dockerclient.NewClient(host)
	if err != nil {
		return nil, err
	}
	return &DockerProvider{
		client:            client,
		dataPath:          dataPath,
		logger:            logger,
		DockerClientProxy: client,
	}, nil
}
