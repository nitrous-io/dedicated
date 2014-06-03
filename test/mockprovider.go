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

func (self *MockProvider) Start(id string) (*provider.ContainerInfo, error) {
	info, _ := self.GetInfo(id)
	if info != nil {
		info.State = provider.ContainerStateRunning
	}
	self.StartCalled = true
	self.ReceivedId = id
	return info, self.StartError
}

func (self *MockProvider) Stop(id string) (*provider.ContainerInfo, error) {
	info, _ := self.GetInfo(id)
	if info != nil {
		info.Host = ""
		info.State = provider.ContainerStateStopped
	}
	self.StopCalled = true
	self.ReceivedId = id
	return info, self.StopError
}

func (self *MockProvider) Restart(id string) (*provider.ContainerInfo, error) {
	info, _ := self.GetInfo(id)
	if info != nil {
		info.State = provider.ContainerStateRunning
	}
	self.RestartCalled = true
	self.ReceivedId = id
	return info, self.RestartError
}

func (self *MockProvider) Terminate(id string) (*provider.ContainerInfo, error) {
	info, _ := self.GetInfo(id)
	if info != nil {
		info.Host = ""
		info.SSHPort = 0
		info.State = ""
	}
	self.TerminateCalled = true
	self.ReceivedId = id
	return info, self.TerminateError
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
