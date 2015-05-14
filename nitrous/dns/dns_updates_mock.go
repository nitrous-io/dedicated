package dns

import (
	"sync"

	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

type MockDNS struct {
	mu sync.Mutex

	OnHostCreateCalls       int
	OnHostStartCalls        int
	OnHostDestroyCalls      int
	OnContainerCreateCalls  int
	OnContainerDestroyCalls int

	OnHostCreateError       error
	OnHostStartError        error
	OnHostDestroyError      error
	OnContainerCreateError  error
	OnContainerDestroyError error
}

func (d *MockDNS) OnHostCreate(h *host.Host) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OnHostCreateCalls++
	return d.OnHostCreateError
}

func (d *MockDNS) OnHostStart(h *host.Host) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OnHostStartCalls++
	return d.OnHostStartError
}

func (d *MockDNS) OnHostDestroy(h *host.Host) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OnHostDestroyCalls++
	return d.OnHostDestroyError
}

func (d *MockDNS) OnContainerCreate(cn *container.Container) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OnContainerCreateCalls++
	return d.OnContainerCreateError
}

func (d *MockDNS) OnContainerDestroy(cn *container.Container) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OnContainerDestroyCalls++
	return d.OnContainerDestroyError
}
