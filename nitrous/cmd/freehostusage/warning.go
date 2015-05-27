package main

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
	"github.com/nitrous-io/nitrous-common/logger"
)

func sendAnnouncementForHostScan(d *db.Database, scan *hostscan.ScanResult, subject, message string) error {
	return d.WithTransaction(func(tx *sqlx.Tx) error {
		hostID := scan.Host.ID
		ann := &announcement.Announcement{
			SubjectTemplate: subject,
			MessageTemplate: message,
			ExpiresAt:       pq.NullTime{scan.NextScan, true},
		}

		err := scan.UpdateHost(tx)
		if err != nil {
			return err
		}

		owner, err := announcement.FindOwnerByHostID(tx, hostID)
		if err != nil {
			return err
		}

		err = announcement.Insert(tx, ann)
		if err != nil {
			return err
		}

		err = announcement.AddHosts(tx, ann.ID, []int64{hostID})
		if err != nil {
			return err
		}

		err = announcement.SendToOwner(ann, owner)
		if err != nil {
			return err
		}

		return nil
	})
}

func scanWarningHosts(d *db.Database, logr *logger.Logger, dryrun bool) {
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
			err := sendAnnouncementForHostScan(d, scan, warningSubject, warningMessage)
			if err != nil {
				logr.Errorf("Error sending warning announcement - %s: %+v.", err, scan)
			}

		case hostscan.StateTerminating:
			if dryrun {
				logr.Infof("Calling sendAnnouncementForHostScan TERMINATING on host %d for %v", scan.Host.ID, scan.State)
				continue
			}
			err := sendAnnouncementForHostScan(d, scan, terminatingSubject, terminatingMessage)
			if err != nil {
				logr.Errorf("Error sending terminating announcement - %s: %+v.", err, scan)
			}

		default:
			logr.Errorf("Unexpected host scan state %s: %+v.", scan.State, scan)
		}
	}
}
