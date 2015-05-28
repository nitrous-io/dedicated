package freehostusage

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
	. "github.com/onsi/gomega"
)

type Message struct {
	Host    *host.Host
	Subject string
	Message string
}

type MockAction struct {
	DB *db.Database

	RefreshHostErr   error
	TerminateHostErr error
	UpdateScanErr    error

	HostScanAnnouncementErr error
	MessageToOwnerErr       error

	RefreshedHosts  []*host.Host
	TerminatedHosts []*host.Host
	UpdatedScans    []*hostscan.ScanResult
	ScanMessages    []*Message
	OwnerMessages   []*Message
}

func (action *MockAction) RefreshHost(h *host.Host) error {
	action.RefreshedHosts = append(action.RefreshedHosts, h)

	return action.RefreshHostErr
}

func (action *MockAction) TerminateHost(h *host.Host) error {
	action.TerminatedHosts = append(action.TerminatedHosts, h)

	return action.TerminateHostErr
}

func (action *MockAction) UpdateHostWithScan(scan *hostscan.ScanResult) error {
	action.UpdatedScans = append(action.UpdatedScans, scan)

	Expect(scan.UpdateHost(action.DB)).To(BeNil())

	return action.UpdateScanErr
}

func (action *MockAction) SendAnnouncementForHostScan(scan *hostscan.ScanResult, subject, message string) error {
	msg := &Message{scan.Host, subject, message}
	action.ScanMessages = append(action.ScanMessages, msg)

	Expect(scan.UpdateHost(action.DB)).To(BeNil())

	return action.HostScanAnnouncementErr
}

func (action *MockAction) SendMessageToHostOwner(h *host.Host, subject, message string) error {
	msg := &Message{h, subject, message}
	action.OwnerMessages = append(action.OwnerMessages, msg)

	return action.MessageToOwnerErr
}
