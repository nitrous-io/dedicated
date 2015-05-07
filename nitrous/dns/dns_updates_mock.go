package dns

import (
	"sync"

	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/nitrous-common/logger"
)

type MockDNS struct {
	mu                      sync.Mutex
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

func (d *MockDNS) OnHostCreate(logger *logger.Logger, h *host.Host) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OnHostCreateCalls++
	return d.OnHostCreateError
}

func (d *MockDNS) OnHostStart(logger *logger.Logger, h *host.Host) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OnHostStartCalls++
	return d.OnHostStartError
}

func (d *MockDNS) OnHostDestroy(logger *logger.Logger, h *host.Host) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OnHostDestroyCalls++
	return d.OnHostDestroyError
}

func (d *MockDNS) OnContainerCreate(logger *logger.Logger, cn *container.Container, h *host.Host) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OnContainerCreateCalls++
	return d.OnContainerCreateError
}

func (d *MockDNS) OnContainerDestroy(logger *logger.Logger, cn *container.Container) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.OnContainerDestroyCalls++
	return d.OnContainerDestroyError
}
