package main

import (
	"os"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/misc/freehostusage"
	"github.com/nitrous-io/nitrous-common/logger"
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

	action := &freehostusage.RealHostAction{d}

	freehostusage.ScanAllFreeHosts(d, logr, action, time.Now(), dryrun)
}
