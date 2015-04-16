package main

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/nitrous-common/logger"
)

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
}

func main() {
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(envvars.DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}

	hosts, err := freetier.ExpiredHosts(dbConn)
	if err != nil {
		logr.FatalExit(err)
	}

	for _, hst := range hosts {
		logr.Infof("Starting shut down on host %d for user %d", hst.ID, hst.UserID)
		err := hostcontrol.Stop(dbConn, logr, hst)
		if err != nil {
			logr.Errorf("Unable to shutdown host %d, err: %v", hst.ID, err)
			continue
		}
		logr.Infof("Successfully shut down host %d for user %d", hst.ID, hst.UserID)
	}
}
