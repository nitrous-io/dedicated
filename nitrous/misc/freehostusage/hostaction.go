package freehostusage

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
)

type HostAction interface {
	TerminateHost(d *db.Database, h *host.Host) error
	UpdateHostWithScan(d *db.Database, scan *hostscan.ScanResult) error
	SendAnnouncementForHostScan(d *db.Database, scan *hostscan.ScanResult, subject, message string) error
}
