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

func (self *MockProvider) List() ([]*provider.ContainerInfo, error) {
	if self.Containers == nil {
		return []*provider.ContainerInfo{}, nil
	}
	return self.Containers, nil
}

func (self *MockProvider) Create(name string, image string, publicKeys []string) (*provider.ContainerInfo, error) {
	self.CreateCalled = true
	self.ReceivedName = name
	self.ReceivedImage = image
	self.ReceivedPublicKeys = publicKeys
	return self.CreateContainerInfo, self.CreateError
}

func (self *MockProvider) GetInfo(id string) (*provider.ContainerInfo, error) {
	for _, info := range self.Containers {
		if info.Id == id {
			return info, nil
		}
	}
	return nil, ErrMockProviderContainerNotFound
}

func (self *MockProvider) Start(id string) error {
	self.StartCalled = true
	self.ReceivedId = id
	return self.StartError
}

func (self *MockProvider) Stop(id string) error {
	self.StopCalled = true
	self.ReceivedId = id
	return self.StopError
}

func (self *MockProvider) Restart(id string) error {
	self.RestartCalled = true
	self.ReceivedId = id
	return self.RestartError
}

func (self *MockProvider) Terminate(id string) error {
	self.TerminateCalled = true
	self.ReceivedId = id
	return self.TerminateError
}

func (self *MockProvider) ListPublicKeys(id string) (map[string]string, error) {
	self.ListPublicKeysCalled = true
	self.ReceivedId = id
	return self.PublicKeys, self.ListPublicKeysError
}

func (self *MockProvider) AddPublicKeys(id string, publicKeys []string) error {
	self.AddPublicKeysCalled = true
	self.ReceivedId = id
	self.ReceivedPublicKeys = publicKeys
	return self.AddPublicKeysError
}

func (self *MockProvider) RemovePublicKey(id string, keyFingerprint string) error {
	self.RemovePublicKeyCalled = true
	self.ReceivedId = id
	self.ReceivedKeyFingerprint = keyFingerprint
	return self.RemovePublicKeyError
}

func (self *MockProvider) IsNotFoundError(err error) bool {
	return err == ErrMockProviderContainerNotFound
}
