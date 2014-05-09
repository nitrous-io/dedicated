package freightertest

import (
	"errors"

	"github.com/nitrous-io/freighter/provider"
)

type MockProvider struct {
	CreateCalled bool

	Name       string
	PublicKeys []string
	WebHookUrl string

	Containers map[string]*provider.ContainerInfo
}

func (p *MockProvider) Create(name string, publicKeys []string, webhookUrl string) (*provider.ContainerInfo, error) {
	p.CreateCalled = true
	p.Name = name
	p.PublicKeys = publicKeys
	p.WebHookUrl = webhookUrl
	return nil, nil
}

func (p *MockProvider) GetInfo(name string) (info *provider.ContainerInfo, err error) {
	info, exists := p.Containers[name]
	if !exists {
		err = errors.New("container not found")
	}
	return
}
