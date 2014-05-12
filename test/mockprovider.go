package freightertest

import (
	"errors"

	"github.com/nitrous-io/freighter/provider"
)

type MockProvider struct {
	CreateCalled        bool
	CreateContainerInfo provider.ContainerInfo
	CreateError         error

	Name       string
	PublicKeys []string

	Containers []provider.ContainerInfo
}

func (p *MockProvider) List() ([]provider.ContainerInfo, error) {
	if p.Containers == nil {
		return []provider.ContainerInfo{}, nil
	}
	return p.Containers, nil
}

func (p *MockProvider) Create(name string, publicKeys []string) (*provider.ContainerInfo, error) {
	p.CreateCalled = true
	p.Name = name
	p.PublicKeys = publicKeys
	return &p.CreateContainerInfo, p.CreateError
}

func (p *MockProvider) GetInfo(name string) (*provider.ContainerInfo, error) {
	for _, info := range p.Containers {
		if info.Name == name {
			return &info, nil
		}
	}
	return nil, errors.New("container not found")
}
