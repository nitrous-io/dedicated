package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/invitecode"
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
			logrus.Fatal(err)
		} else {
			fmt.Println(ic.Code)
		}
	}
}
