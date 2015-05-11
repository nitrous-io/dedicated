package main

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/nitrous-common/httpclient"
	"github.com/nitrous-io/nitrous-common/logger"
)

// FIXME: Stub messages
const (
	warningSubject = `Workspace Inactivity Notice`
	warningMessage = `You host {{.Slug}} is inactive and will be terminated in a week if there is no usage`

	terminatingSubject = `Workspace termination due to inactivity`
	terminatingMessage = `Your host {{.Slug}} will be terminated in 3 days.`

	terminatedSubject = `Workspace Termination Notice`
	teminatedMessage  = `Your workspace {{.Slug}} has been terminated due to inactivity`
)

var (
	d    *db.Database
	logr *logger.Logger
)

func refreshHost(h *host.Host) error {
	reqUrl := fmt.Sprintf("https://%s/api/v0/maintenance/user_refresh/%d", envvars.WebHostname, h.UserID)
	c := httpclient.New("")
	resp, _ := c.PostForm(reqUrl, &url.Values{"trust_key": []string{envvars.TrustKey}})
	return httpclient.CheckResponse(resp, http.StatusOK)
}

func terminateHost(h *host.Host) error {
	// Sanity checks. should never get this
	// back from hostscan.FindTerminatingHosts
	if h.HourlyPrice > 0 {
		logr.Errorf("Sanity check failed: cannot terminate non-free host: %+v", h)
		return fmt.Errorf("host is not free")
	}

	if !h.TerminatingAt.Valid || h.TerminatingAt.Time.Before(time.Now()) {
		logr.Errorf("Sanity check failed: host.terminating_at is not valid - %+v: %+v", h.TerminatingAt, h)
		return fmt.Errorf("host.terminating_at is not valid")
	}

	logr.Infof("Terminating host %d for user %d", h.ID, h.UserID)
	timeStart := time.Now()

	headers := http.Header{}
	headers.Set(constants.NitrousUUIDHeader, "hostcontrol")

	err := d.WithTransaction(func(tx *sqlx.Tx) error {
		return hostcontrol.Terminate(tx, logr, h, headers)
	})
	if err != nil {
		logr.Errorf("Error terminating host - %s: %+v", err, h)
		return err
	}

	logr.Infof("Successfully terminated host %d for user %d in %s", h.ID, h.UserID, time.Since(timeStart))

	err = refreshHost(h)
	if err != nil {
		logr.Errorf("Unable to send user_refresh for user %d, err: %+v", h.UserID, err)
		// continue
	}

	owner, err := announcement.FindOwnerByHostID(d, h.ID)
	if err != nil {
		logr.Errorf("Error finding host ownder detail - %s: %+v", err, h)
		return err
	}

	ann := &announcement.Announcement{
		SubjectTemplate: terminatedSubject,
		MessageTemplate: teminatedMessage,
	}

	// Send without saving announcement
	err = announcement.SendToOwner(ann, owner)
	if err != nil {
		logr.Errorf("Error sending termination notice to host owner - %s: %+v", err, owner)
		return err
	}

	return nil
}

func sendAnnouncementForHostScan(scan *hostscan.ScanResult, subject, message string) error {
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

func scanWarningHosts() {
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
				logr.Errorf("Error updating scan result - %s: %+v.", err, scan)
			}

		case hostscan.StateOneWeekNotice:
			err := sendAnnouncementForHostScan(scan, warningSubject, warningMessage)
			if err != nil {
				logr.Errorf("Error sending warning announcement - %s: %+v.", err, scan)
			}

		case hostscan.StateTerminating:
			err := sendAnnouncementForHostScan(scan, terminatingSubject, terminatingMessage)
			if err != nil {
				logr.Errorf("Error sending terminating announcement - %s: %+v.", err, scan)
			}

		default:
			logr.Errorf("Unexpected host scan state %s: %+v.", scan.State, scan)
		}
	}
}

func terminateInactiveFreeHosts() {
	hosts, err := hostscan.FindTerminatingHosts(d, time.Now())
	if err != nil {
		logr.FatalfExit("Failed to query for teminating hosts")
	}

	logr.Infof("Found %d free hosts to terminate", len(hosts))

	for _, h := range hosts {
		err := terminateHost(h)
		if err != nil {
			logr.Errorf("Error terminating host: %s", err)
		}
	}
}

func main() {
	var err error

	logr = &logger.Logger{Level: logger.LogLevelDebug}

	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(envvars.DBEnvVar, "is not defined")
	}

	d, err = db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalfExit("db.NewDatabase() failed with '%s'", err)
	}

	if envvars.WebHostname == "" {
		logr.FatalfExit("WEB_HOSTNAME is not defined")
	}

	if envvars.TrustKey == "" {
		logr.FatalfExit("TRUST_KEY is not defined")
	}

	scanWarningHosts()
	terminateInactiveFreeHosts()
}
