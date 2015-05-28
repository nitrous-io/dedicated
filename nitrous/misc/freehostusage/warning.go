package freehostusage

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
	"github.com/nitrous-io/nitrous-common/logger"
)

const (
	warningSubject = `Workspace termination warning - 7 day notice`
	warningMessage = `It looks like you have not been using your Nitrous workspace during the past month.

Your workspace {{ .Slug }} will be terminated in 7 days if you are not actively developing an application.

If you no longer need your workspace and its contents, no further action is required.

Should you wish to keep your workspace, simply start using it regularly.

Please contact us with any questions at support@nitrous.io .
`

	terminatingSubject = `Workspace termination warning - final notice`
	terminatingMessage = `Your workspace {{.Slug}} will be terminated in 3 days.

You can upgrade to a paid plan to keep it running.  If you do not wish to upgrade, please be sure to backup your files prior to termination.`

	terminatedSubject = `Workspace termination notice`
	terminatedMessage = `Your Nitrous Pro workspace has been terminated. You will no longer be able to access this workspace.`
)

func ScanWarningHosts(d *db.Database, logr *logger.Logger, dryrun bool) {
	scans, errs := hostscan.ScanHosts(d, time.Now())

	logr.Infof("Host scan found %d hosts and %d errors.", len(scans), len(errs))

	for _, err := range errs {
		logr.Errorf("Error scanning host: %s.", err)
	}

	for _, scan := range scans {
		switch scan.State {
		case hostscan.StateActive, hostscan.StateGracePeriod:
			if dryrun {
				logr.Infof("Calling scan.UpdateHost on host %d for %v", scan.Host.ID, scan.State)
				continue
			}
			err := scan.UpdateHost(d)
			if err != nil {
				// ignore
				logr.Errorf("Error updating scan result - %s: %+v.", err, scan)
			}

		case hostscan.StateOneWeekNotice:
			if dryrun {
				logr.Infof("Calling sendAnnouncementForHostScan WARNING on host %d for %v", scan.Host.ID, scan.State)
				continue
			}
			err := hostscan.SendAnnouncementForHostScan(d, scan, warningSubject, warningMessage)
			if err != nil {
				logr.Errorf("Error sending warning announcement - %s: %+v.", err, scan)
			}

		case hostscan.StateTerminating:
			if dryrun {
				logr.Infof("Calling sendAnnouncementForHostScan TERMINATING on host %d for %v", scan.Host.ID, scan.State)
				continue
			}
			err := hostscan.SendAnnouncementForHostScan(d, scan, terminatingSubject, terminatingMessage)
			if err != nil {
				logr.Errorf("Error sending terminating announcement - %s: %+v.", err, scan)
			}

		default:
			logr.Errorf("Unexpected host scan state %s: %+v.", scan.State, scan)
		}
	}
}
