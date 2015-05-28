package freehostusage

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

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
	terminatedSubject = `Workspace termination notice`
	terminatedMessage = `Your Nitrous Pro workspace has been terminated. You will no longer be able to access this workspace.`
)

func refreshHost(h *host.Host) error {
	reqUrl := fmt.Sprintf("https://%s/api/v0/maintenance/user_refresh/%d", envvars.WebHostname, h.UserID)
	c := httpclient.New("")
	resp, _ := c.PostForm(reqUrl, &url.Values{"trust_key": []string{envvars.TrustKey}})
	return httpclient.CheckResponse(resp, http.StatusOK)
}

func TerminateFreeHost(d *db.Database, h *host.Host, logr *logger.Logger, dryrun bool) error {
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

	// FIXME until this command has been verified on production, we only
	// set host state to `orphan` for now so that it is hidden from the user
	// and no data is removed
	// err := d.WithTransaction(func(tx *sqlx.Tx) error {
	//   return hostcontrol.Terminate(tx, logr, h, headers, initiatorID)
	// })

	_, err := d.Exec(`UPDATE hosts SET state = $1 WHERE id = $2;`, host.StateOrphan, h.ID)
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

	err = announcement.SendMessageToHostOwner(d, h.ID, terminatedSubject, terminatedMessage)
	if err != nil {
		logr.Errorf("Unable to send terminate notice for user %d, err: %+v", h.UserID, err)
		// continue
	}

	return nil
}

func TerminateInactiveFreeHosts(d *db.Database, logr *logger.Logger, dryrun bool) {
	hosts, err := hostscan.FindTerminatingHosts(d, time.Now())
	if err != nil {
		logr.FatalfExit("Failed to query for teminating hosts")
	}

	logr.Infof("Found %d free hosts to terminate", len(hosts))

	for _, h := range hosts {
		err := TerminateFreeHost(d, h, logr, dryrun)
		if err != nil {
			logr.Errorf("Error terminating host: %s", err)
		}
	}
}
