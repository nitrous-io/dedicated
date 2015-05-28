package freehostusage

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
)

type HostAction interface {
	RefreshHost(h *host.Host) error
	TerminateHost(h *host.Host) error
	UpdateHostWithScan(scan *hostscan.ScanResult) error

	// Update scan result to database and send announcement to owner,
	// all in single db transaction
	SendAnnouncementForHostScan(scan *hostscan.ScanResult, subject, message string) error

	// Send email to owner only. No interaction with database
	SendMessageToHostOwner(h *host.Host, subject, message string) error
}

type RealHostAction struct {
	DB *db.Database
}

func (action *RealHostAction) RefreshHost(h *host.Host) error {
	return RefreshHost(h)
}

func (action *RealHostAction) TerminateHost(h *host.Host) error {
	// FIXME until this command has been verified on production, we only
	// set host state to `orphan` for now so that it is hidden from the user
	// and no data is removed
	// err := d.WithTransaction(func(tx *sqlx.Tx) error {
	//   return hostcontrol.Terminate(tx, logr, h, headers, initiatorID)
	// })

	_, err := action.DB.Exec(`UPDATE hosts SET state = $1 WHERE id = $2;`, host.StateOrphan, h.ID)
	return err
}

func (action *RealHostAction) UpdateHostWithScan(scan *hostscan.ScanResult) error {
	return scan.UpdateHost(action.DB)
}

func (action *RealHostAction) SendAnnouncementForHostScan(scan *hostscan.ScanResult, subject, message string) error {
	return hostscan.SendAnnouncementForHostScan(action.DB, scan, warningSubject, warningMessage)
}

func (action *RealHostAction) SendMessageToHostOwner(h *host.Host, subject, message string) error {
	return announcement.SendMessageToHostOwner(action.DB, h.ID, subject, message)
}
