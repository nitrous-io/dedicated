package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func main() {
	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatal(envvars.DBEnvVar, "is not defined")
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatal(err)
	}

	logrus.Debug("Removing tokens")
	if err := token.DeleteExpiredTokens(dbConn); err != nil {
		logrus.Fatal(err)
	}
}
