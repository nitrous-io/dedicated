package freehostusage

import (
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
)

type Message struct {
	Host    *host.Host
	Subject string
	Message string
}

type MockAction struct {
	RefreshHostErr   error
	TerminateHostErr error
	UpdateScanErr    error

	HostScanAnnouncementErr error
	MessageToOwnerErr       error

	RefreshedHosts  []*host.Host
	TerminatedHosts []*host.Host
	UpdatedScans    []*hostscan.ScanResult
	SentMessages    []*Message
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

	return action.UpdateScanErr
}

func (action *MockAction) SendAnnouncementForHostScan(scan *hostscan.ScanResult, subject, message string) error {
	msg := &Message{scan.Host, subject, message}
	action.SentMessages = append(action.SentMessages, msg)
	action.UpdatedScans = append(action.UpdatedScans, scan)

	return action.HostScanAnnouncementErr
}

func (action *MockAction) SendMessageToHostOwner(h *host.Host, subject, message string) error {
	msg := &Message{h, subject, message}
	action.SentMessages = append(action.SentMessages, msg)

	return action.MessageToOwnerErr
}
