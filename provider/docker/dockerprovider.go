package docker

import (
	"github.com/nitrous-io/freighter/provider"
)

type DockerProvider struct{}

func (p *DockerProvider) List() ([]*provider.ContainerInfo, error) {
	return nil, nil
}

func (p *DockerProvider) Create(name string, image string, publicKeys []string) (*provider.ContainerInfo, error) {
	return nil, nil
}

func (p *DockerProvider) GetInfo(name string) (*provider.ContainerInfo, error) {
	return nil, nil
}

func (p *DockerProvider) Terminate(id string) error {
	return nil
}

func (p *DockerProvider) Start(id string) error {
	return nil
}

func (p *DockerProvider) Stop(id string) error {
	return nil
}

func (p *DockerProvider) Restart(id string) error {
	return nil
}
