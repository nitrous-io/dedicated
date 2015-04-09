package docker

import (
	"errors"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/util"
)

var (
	GatewayFromIPOutput = gatewayFromIPOutput
)

func (p *DockerProvider) Gateway() string {
	return p.gateway
}

func (p *DockerProvider) ConfigFromContainer(cnr *dockerclient.Container) *provider.ContainerConfig {
	return p.configFromContainer(cnr)
}

func (p *DockerProvider) MkContainerConfig(cfg *provider.ContainerConfig, isNitrousImage bool) (*dockerclient.Config, error) {
	return p.mkContainerConfig(cfg, isNitrousImage)
}

func (p *DockerProvider) MkHostConfig(c *dockerclient.Container, cfg *provider.ContainerConfig, isNitrousImage bool) (*dockerclient.HostConfig, error) {
	return p.mkHostConfig(c, cfg, isNitrousImage)
}

// Client exposes private client in tests
func (p *DockerProvider) Client() *dockerclient.Client {
	return p.client
}

func (p *DockerProvider) IsNitrousImage(fullName string) bool {
	return p.isNitrousImage(fullName)
}

func (p *DockerProvider) InspectRemoteImage(location string) (*provider.ImageInfo, error) {
	return p.inspectRemoteImage(location)
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
