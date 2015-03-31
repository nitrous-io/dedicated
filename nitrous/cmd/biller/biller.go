package main

import (
	"os"
	"strconv"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/appconfig"
	"github.com/nitrous-io/dedicated/nitrous/biller"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/stripe/stripe-go"
)

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}

	appconfig.CheckSendgridVariables()

	stripe.Key = appconfig.StripeSecretKey
	if stripe.Key == "" {
		logr.FatalExit("STRIPE_SECRET_KEY environment variable not set")
	}
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
			logr.FatalExit(err)
		}
		us, err := user.FindByID(dbConn, uID)
		if err != nil {
			logr.FatalExit(err)
		}
		if us == nil {
			logr.FatalExit("biller: USER_ID is not a valid user")
		}

		pS, pE := biller.StartEndTimeOrDefault(startTime, endTime)
		b, err := biller.New(dbConn, logr, us, &pS, &pE)
		if err != nil {
			logr.FatalExit(err)
		}
		if err := b.Process(); err != nil {
			logr.FatalExit(err)
		}
		return
	}

	biller.ProcessForAllUsers(dbConn, logr, startTime, endTime)
}

func extractUnixTime(val string) time.Time {
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		logr.FatalExit(err)
	}
	return time.Unix(i, 0)
}
