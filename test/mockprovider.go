package freightertest

import (
	"errors"

	"github.com/nitrous-io/freighter/provider"
)

type MockProvider struct {
	CreateCalled        bool
	CreateContainerInfo *provider.ContainerInfo
	CreateError         error

	TerminateCalled bool
	TerminateError  error

	Id         string
	Name       string
	Image      string
	PublicKeys []string

	Containers []*provider.ContainerInfo
}

func (p *MockProvider) List() ([]*provider.ContainerInfo, error) {
	if p.Containers == nil {
		return []*provider.ContainerInfo{}, nil
	}
	return p.Containers, nil
}

func (p *MockProvider) Create(name string, image string, publicKeys []string) (*provider.ContainerInfo, error) {
	p.CreateCalled = true
	p.Name = name
	p.Image = image
	p.PublicKeys = publicKeys
	return p.CreateContainerInfo, p.CreateError
}

func (p *MockProvider) GetInfo(id string) (*provider.ContainerInfo, error) {
	for _, info := range p.Containers {
		if info.Id == id {
			return info, nil
		}
	}
	return nil, errors.New("container not found")
}

func (p *MockProvider) Terminate(id string) error {
	p.TerminateCalled = true
	p.Id = id
	return p.TerminateError
}
