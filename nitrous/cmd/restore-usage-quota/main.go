/*
This command restores the free usage remaining of users to full upon the
anniversary of their free host being created.

E.g. a free host created on 1 Jan will have its 50 free hours restored on 31 Jan
(30 days from 1 Jan).
*/
package main

import (
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
}

func main() {
	if os.Getenv("LOG_LEVEL") != "" {
		if lvl, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL")); err != nil {
			logrus.SetLevel(lvl)
		}
	}

	logrus.Debug("Restoring usage quotas..")

	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatalf("%v is not defined", envvars.DBEnvVar)
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("failed to connect to database, err: %v", err)
	}

	numRestored, err := user.RestoreUsageQuotas(dbConn, time.Now())
	if err != nil {
		logrus.Fatalf("failed to restore usage quotas, err: %v", err)
	}

	logrus.Debugf("Restored usage quotas for %d users. Exiting.", numRestored)
}
