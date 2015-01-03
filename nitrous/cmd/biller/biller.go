package main

import (
	"os"
	"strconv"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/biller"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/stripe/stripe-go"
)

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}

	envvars.CheckSendgridVariables()

	stripe.Key = envvars.StripeSecretKey
	if stripe.Key == "" {
		panic("STRIPE_SECRET_KEY environment variable not set")
	}
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

	var startTime, endTime *time.Time
	if startTimeEnv := os.Getenv("USAGE_START"); len(startTimeEnv) != 0 {
		val := extractUnixTime(startTimeEnv)
		startTime = &val
	}
	if endTimeEnv := os.Getenv("USAGE_END"); len(endTimeEnv) != 0 {
		val := extractUnixTime(endTimeEnv)
		endTime = &val
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
