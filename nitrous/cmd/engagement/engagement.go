package main

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/engagement"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
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

	envvars.CheckSendgrid()
	logr.Info("engagement: Starting Activation Emails")
	users, err := engagement.ActivationEmailCandidates(dbConn)
	if err != nil {
		logr.Fatalf("engagement: Error fetching ActivationEmailCandidates, err: %v", err)
	}
	for _, user := range users {
		logr.Infof("engagement: Sending activation email to %d", user.ID)
		if err := engagement.SendActivationEmail(dbConn, user); err != nil {
			logr.Errorf("engagement: Error sending activation email to %d, err: %v", user.ID, err)
		}
	}

	logr.Info("engagement: Starting Get Started Emails")
	users, err = engagement.GetStartedEmailCandidates(dbConn)
	if err != nil {
		logr.Fatalf("engagement: Error fetching GetStartedEmailCandidates, err: %v", err)
	}
	for _, user := range users {
		logr.Infof("engagement: Sending get started email to %d", user.ID)
		if err := engagement.SendGetStartedEmail(dbConn, user); err != nil {
			logr.Fatalf("engagement: Error sending get started email to %d, err: %v", user.ID, err)
		}
	}
}
