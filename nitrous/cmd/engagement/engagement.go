package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/engagement"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
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

	envvars.CheckSendgrid()
	logrus.Info("engagement: Starting Activation Emails")
	users, err := engagement.ActivationEmailCandidates(dbConn)
	if err != nil {
		logrus.Fatalf("engagement: Error fetching ActivationEmailCandidates, err: %v", err)
	}
	for _, user := range users {
		logrus.Infof("engagement: Sending activation email to %d", user.ID)
		if err := engagement.SendActivationEmail(dbConn, user); err != nil {
			logrus.Errorf("engagement: Error sending activation email to %d, err: %v", user.ID, err)
		}
	}

	logrus.Info("engagement: Starting Get Started Emails")
	users, err = engagement.GetStartedEmailCandidates(dbConn)
	if err != nil {
		logrus.Fatalf("engagement: Error fetching GetStartedEmailCandidates, err: %v", err)
	}
	for _, user := range users {
		logrus.Infof("engagement: Sending get started email to %d", user.ID)
		if err := engagement.SendGetStartedEmail(dbConn, user); err != nil {
			logrus.Fatalf("engagement: Error sending get started email to %d, err: %v", user.ID, err)
		}
	}
}
