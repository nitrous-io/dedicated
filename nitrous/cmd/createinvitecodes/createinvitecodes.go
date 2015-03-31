package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/appconfig"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/invitecode"
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

	count := 1
	if len(os.Args) >= 2 {
		count, err = strconv.Atoi(os.Args[1])
		if err != nil {
			count = 1
		}
	}

	for i := 1; i <= count; i++ {
		ic, err := invitecode.GenerateInvite(dbConn)
		if err != nil {
			logr.FatalExit(err)
		} else {
			fmt.Println(ic.Code)
		}
	}
}
