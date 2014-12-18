package main

import (
	"os"
	"strconv"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/usagerecorder"
	"github.com/nitrous-io/nitrous-common/logger"
)

const DBEnvVar = "NITROUS_PG_URL"

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
}

func main() {
	dbURL := os.Getenv(DBEnvVar)
	if dbURL == "" {
		logr.FatalExit(DBEnvVar, "is not defined")
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
	uR := usagerecorder.New(dbConn, logr, startTime, endTime)
	uR.ProcessUsages()
}

func extractUnixTime(val string) time.Time {
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		logr.FatalExit(err)
	}
	return time.Unix(i, 0)
}
