package main

import (
	"os"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/usagerecorder"
)

const DBEnvVar = "NITROUS_PG_URL"

func init() {
	logrus.SetLevel(logrus.DebugLevel)
}

func main() {
	dbURL := os.Getenv(DBEnvVar)
	if dbURL == "" {
		logrus.Fatal(DBEnvVar, "is not defined")
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
	uR := usagerecorder.New(dbConn, startTime, endTime)
	uR.ProcessUsages()
}

func extractUnixTime(val string) time.Time {
	i, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		logrus.Fatal(err)
	}
	return time.Unix(i, 0)
}
