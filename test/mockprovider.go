package test

import (
	"errors"

	"github.com/nitrous-io/freighter/provider"
)

var ErrMockProviderContainerNotFound = errors.New("no such container")

type MockProvider struct {
	CreateCalled        bool
	CreateContainerInfo *provider.ContainerInfo
	CreateError         error

	TerminateCalled bool
	TerminateError  error

	StartCalled bool
	StartError  error

	StopCalled bool
	StopError  error

	RestartCalled bool
	RestartError  error

	ListPublicKeysCalled bool
	ListPublicKeysError  error

	AddPublicKeysCalled bool
	AddPublicKeysError  error

	RemovePublicKeyCalled bool
	RemovePublicKeyError  error

	ReceivedId             string
	ReceivedName           string
	ReceivedImage          string
	ReceivedPublicKeys     []string
	ReceivedKeyFingerprint string

	Containers []*provider.ContainerInfo
	PublicKeys map[string]string
}

func (p *MockProvider) List() ([]*provider.ContainerInfo, error) {
	if p.Containers == nil {
		return []*provider.ContainerInfo{}, nil
	}
	return p.Containers, nil
}

func (p *MockProvider) Create(name string, image string, publicKeys []string) (*provider.ContainerInfo, error) {
	p.CreateCalled = true
	p.ReceivedName = name
	p.ReceivedImage = image
	p.ReceivedPublicKeys = publicKeys
	return p.CreateContainerInfo, p.CreateError
}

func (p *MockProvider) GetInfo(id string) (*provider.ContainerInfo, error) {
	for _, info := range p.Containers {
		if info.Id == id {
			return info, nil
		}
	}
	return nil, ErrMockProviderContainerNotFound
}

func (p *MockProvider) Start(id string) error {
	p.StartCalled = true
	p.ReceivedId = id
	return p.StartError
}

func (p *MockProvider) Stop(id string) error {
	p.StopCalled = true
	p.ReceivedId = id
	return p.StopError
}

func (p *MockProvider) Restart(id string) error {
	p.RestartCalled = true
	p.ReceivedId = id
	return p.RestartError
}

func (p *MockProvider) Terminate(id string) error {
	p.TerminateCalled = true
	p.ReceivedId = id
	return p.TerminateError
}

func (p *MockProvider) ListPublicKeys(id string) (map[string]string, error) {
	p.ListPublicKeysCalled = true
	p.ReceivedId = id
	return p.PublicKeys, p.ListPublicKeysError
}

func (p *MockProvider) AddPublicKeys(id string, publicKeys []string) error {
	p.AddPublicKeysCalled = true
	p.ReceivedId = id
	p.ReceivedPublicKeys = publicKeys
	return p.AddPublicKeysError
}

func (p *MockProvider) RemovePublicKey(id string, keyFingerprint string) error {
	p.RemovePublicKeyCalled = true
	p.ReceivedId = id
	p.ReceivedKeyFingerprint = keyFingerprint
	return p.RemovePublicKeyError
}

func (p *MockProvider) IsNotFoundError(err error) bool {
	return err == ErrMockProviderContainerNotFound
}
