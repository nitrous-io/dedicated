package main

import (
	"github.com/nitrous-io/dedicated/nitrous/appconfig"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/nitrous-common/logger"
)

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
}

func main() {
	dbURL := appconfig.NitrousPgUrl
	if dbURL == "" {
		logr.FatalExit(appconfig.DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logr.FatalExit(err)
	}

	logr.Debug("Removing tokens")
	if err := token.DeleteExpiredTokens(dbConn); err != nil {
		logr.FatalExit(err)
	}
}
