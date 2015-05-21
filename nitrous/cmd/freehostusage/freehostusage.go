package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hostscan"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/nitrous-common/httpclient"
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

var (
	d           *db.Database
	logr        *logger.Logger
	initiatorID int64

	dryrun = true
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
	if !h.Free {
		logr.Errorf("Sanity check failed: cannot terminate non-free host: %+v", h)
		return fmt.Errorf("host is not free")
	}

	if !h.TerminatingAt.Valid || h.TerminatingAt.Time.Before(time.Now()) {
		logr.Errorf("Sanity check failed: host.terminating_at is not valid - %+v: %+v", h.TerminatingAt, h)
		return fmt.Errorf("host.terminating_at is not valid")
	}

	logr.Infof("Terminating host %d for user %d", h.ID, h.UserID)

	if dryrun {
		return nil
	}

	timeStart := time.Now()

	headers := http.Header{}
	headers.Set(constants.NitrousUUIDHeader, "hostcontrol")

	err := d.WithTransaction(func(tx *sqlx.Tx) error {
		// FIXME until this command has been verified on production, we only
		// set host state to `orphan` for now so that it is hidden from the user
		// and no data is removed
		_, err := tx.Exec(`UPDATE hosts SET state = $1 WHERE id = $2;`, host.StateOrphan, h.ID)
		return err
		// return hostcontrol.Terminate(tx, logr, h, headers, initiatorID)
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
		logr.Errorf("Error finding host owner detail - %s: %+v", err, h)
		return err
	}

	ann := &announcement.Announcement{
		SubjectTemplate: terminatedSubject,
		MessageTemplate: terminatedMessage,
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
			err := sendAnnouncementForHostScan(scan, warningSubject, warningMessage)
			if err != nil {
				logr.Errorf("Error sending warning announcement - %s: %+v.", err, scan)
			}

		case hostscan.StateTerminating:
			if dryrun {
				logr.Infof("Calling sendAnnouncementForHostScan TERMINATING on host %d for %v", scan.Host.ID, scan.State)
				continue
			}
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

	if os.Args[len(os.Args)-1] == "--live-run" {
		dryrun = false
		logr.Warnf("Live Run!")
	}

	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(envvars.DBEnvVar, "is not defined")
	}

	d, err = db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalfExit("db.NewDatabase() failed with '%s'", err)
	}

	if envvars.WebHostname == "" {
		logr.FatalExit("WEB_HOSTNAME is not defined")
	}

	if envvars.TrustKey == "" {
		logr.FatalExit("TRUST_KEY is not defined")
	}

	if envvars.ProvisionerUser == "" {
		logr.FatalExit("PROVISIONER_USER is not defined")
	}

	err = d.QueryRow(`SELECT id FROM users WHERE username = $1`, envvars.ProvisionerUser).Scan(&initiatorID)
	if err != nil {
		logr.FatalfExit("Failed to locate the ID of the PROVISIONER_USER. Err: '%s'", err)
	}

	scanWarningHosts()
	terminateInactiveFreeHosts()
}
