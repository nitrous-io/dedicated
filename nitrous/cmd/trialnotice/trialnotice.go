package main

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
)

var (
	dryrun = true

	dbConn *db.Database

	// Be specific about period range so email is not sent twice to user with overlapping
	// trial ending period. Eg. if the range is []int{2*24, 3*24}, send to subs with trial ending
	// in less than 3 days but not more than 2 days.
	firstEmailDaysRange  = []int{2 * 24, 3 * 24}
	secondEmailDaysRange = []int{0, 1 * 24}
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	for _, arg := range os.Args {
		if arg == "--live-run" {
			dryrun = false
		}
	}
	if dryrun {
		logrus.Warnf("Operating as a dry-run, no destructive actions will occur")
	} else {
		logrus.Warnf("--live-run specified, will be performing destructive actions")
	}

	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatal(envvars.DBEnvVar, "is not defined")
	}
	var err error
	dbConn, err = db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("db.NewDatabase() failed with '%s'", err)
	}
}

func main() {
	subs1, err := subscription.FindAllTrialEnding(dbConn, firstEmailDaysRange[0], firstEmailDaysRange[1])
	if err != nil {
		logrus.Fatalf("subscription.FindAllFreeTrialEnding() days left %v failed with '%v'", firstEmailDaysRange[1], err)
	}
	for _, s := range subs1 {
		go func() {
			if err := subscription.SendTrialEndingEmail(dbConn, s, dryrun); err != nil {
				logrus.Errorf("subscription.SendTrialEndingEmail err: %v", err)
			}
		}()
	}

	subs2, err := subscription.FindAllTrialEnding(dbConn, secondEmailDaysRange[0], secondEmailDaysRange[1])
	if err != nil {
		logrus.Fatalf("subscription.FindAllFreeTrialEnding() days left %v failed with '%v'", secondEmailDaysRange[1], err)
	}
	for _, s := range subs2 {
		go func() {
			if err := subscription.SendTrialEndingEmail(dbConn, s, dryrun); err != nil {
				logrus.Errorf("subscription.SendTrialEndingEmail err: %v", err)
			}
		}()
	}

}
