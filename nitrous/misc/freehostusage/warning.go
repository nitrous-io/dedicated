package freehostusage

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
	"github.com/nitrous-io/nitrous-common/logger"
)

func ScanWarningHosts(d *db.Database, logr *logger.Logger, action HostAction, now time.Time, dryrun bool) {
	scans, errs := hostscan.ScanHosts(d, now)

	logr.Infof("Host scan found %d hosts and %d errors.", len(scans), len(errs))

	for _, err := range errs {
		logr.Errorf("Error scanning host: %s.", err)
	}

	for _, scan := range scans {
		switch scan.State {
		case hostscan.StateActive, hostscan.StateGracePeriod, hostscan.StateSkip:
			if dryrun {
				logr.Infof("Calling scan.UpdateHost on host %d for %v", scan.Host.ID, scan.State)
				continue
			}

			err := action.UpdateHostWithScan(scan)
			if err != nil {
				// ignore
				logr.Errorf("Error updating scan result - %s: %+v.", err, scan)
			}

		case hostscan.StateOneWeekNotice:
			if dryrun {
				logr.Infof("Calling sendAnnouncementForHostScan WARNING on host %d for %v", scan.Host.ID, scan.State)
				continue
			}
			err := action.SendAnnouncementForHostScan(scan, WarningSubject, WarningMessage)
			if err != nil {
				logr.Errorf("Error sending warning announcement - %s: %+v.", err, scan)
			}

		case hostscan.StateTerminating:
			if dryrun {
				logr.Infof("Calling sendAnnouncementForHostScan TERMINATING on host %d for %v", scan.Host.ID, scan.State)
				continue
			}
			err := action.SendAnnouncementForHostScan(scan, TerminatingSubject, TerminatingMessage)
			if err != nil {
				logr.Errorf("Error sending terminating announcement - %s: %+v.", err, scan)
			}

		default:
			logr.Errorf("Unexpected host scan state %s: %+v.", scan.State, scan)
		}
	}
}
