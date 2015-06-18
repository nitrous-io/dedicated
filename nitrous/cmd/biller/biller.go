package main

import (
	"os"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/biller"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	stripe "github.com/stripe/stripe-go"
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)

	envvars.CheckSendgrid()

	stripe.Key = envvars.StripeSecretKey
	if stripe.Key == "" {
		logrus.Fatal("STRIPE_SECRET_KEY environment variable not set")
	}
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

	var startTime, endTime *time.Time
	if startTimeEnv := os.Getenv("USAGE_START"); len(startTimeEnv) != 0 {
		val := extractUnixTime(startTimeEnv)
		startTime = &val
	}
	if endTimeEnv := os.Getenv("USAGE_END"); len(endTimeEnv) != 0 {
		val := extractUnixTime(endTimeEnv)
		endTime = &val
	}
	if targetUserID := os.Getenv("USER_ID"); len(targetUserID) != 0 {
		uID, err := strconv.ParseInt(targetUserID, 10, 64)
		if err != nil {
			logrus.Fatal(err)
		}
		us, err := user.FindByID(dbConn, uID)
		if err != nil {
			logrus.Fatal(err)
		}
		if us == nil {
			logrus.Fatal("biller: USER_ID is not a valid user")
		}

		pS, pE := biller.StartEndTimeOrDefault(startTime, endTime)
		b, err := biller.New(dbConn, us, &pS, &pE)
		if err != nil {
			logrus.Fatal(err)
		}
		if err := b.Process(); err != nil {
			logrus.Fatal(err)
		}
		return
	}

	biller.ProcessForAllUsers(dbConn, startTime, endTime)
}

func extractUnixTime(val string) time.Time {
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		logrus.Fatal(err)
	}
	return time.Unix(i, 0)
}
