package freightertest

import (
	"errors"

	"github.com/nitrous-io/freighter/provider"
)

type MockProvider struct {
	CreateCalled        bool
	CreateContainerInfo *provider.ContainerInfo
	CreateError         error

	Name       string
	PublicKeys []string

	Containers map[string]*provider.ContainerInfo
}

func (p *MockProvider) Create(name string, publicKeys []string) (*provider.ContainerInfo, error) {
	p.CreateCalled = true
	p.Name = name
	p.PublicKeys = publicKeys
	return p.CreateContainerInfo, p.CreateError
}

func (p *MockProvider) GetInfo(name string) (info *provider.ContainerInfo, err error) {
	info, exists := p.Containers[name]
	if !exists {
		err = errors.New("container not found")
	}
	return
}
