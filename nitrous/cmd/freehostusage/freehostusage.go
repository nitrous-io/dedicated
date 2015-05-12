package main

import (
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
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

func main() {
	logr := &logger.Logger{Level: logger.LogLevelDebug}

	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(envvars.DBEnvVar, "is not defined")
	}

	d, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalfExit("db.NewDatabase() failed with '%s'", err)
	}

	scans, errs := hostscan.ScanHosts(d, time.Now())

	logr.Infof("Host scan found %d hosts and %d errors.", len(scans), len(errs))

	for _, err := range errs {
		logr.Errorf("Error scanning host: %s.", err)
	}

	for _, scan := range scans {
		switch scan.State {
		case hostscan.StateActive, hostscan.StateGracePeriod:
			err := scan.UpdateHost(d)
			if err != nil {
				// ignore
				logr.Errorf("error updating scan result - %s: %+v.", err, scan)
			}

		case hostscan.StateOneWeekNotice:
			err := sendAnnouncementForHostScan(d, scan, warningSubject, warningMessage)
			if err != nil {
				logr.Errorf("error sending warning announcement - %s: %+v.", err, scan)
			}

		case hostscan.StateTerminating:
			err := sendAnnouncementForHostScan(d, scan, terminatingSubject, terminatingMessage)
			if err != nil {
				logr.Errorf("error sending terminating announcement - %s: %+v.", err, scan)
			}
		}
	}
}
