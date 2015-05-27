package main

import (
	"os"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
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

func main() {
	var (
		err         error
		d           *db.Database
		logr        *logger.Logger
		initiatorID int64

		dryrun = true
	)

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

	scanWarningHosts(d, logr, dryrun)
	terminateInactiveFreeHosts(d, logr, dryrun)
}
