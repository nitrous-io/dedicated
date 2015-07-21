package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
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

	var ids []int64
	err = dbConn.Select(&ids, `SELECT id FROM subscriptions WHERE slug IS NULL;`)
	if err != nil {
		logrus.Fatal(err)
	}

	for index, id := range ids {
		if index != 0 && index%100 == 0 {
			logrus.Infof("Completed %d", index)
		}

		slug, err := securerandom.Base64(8)
		if err != nil {
			logrus.Fatal(err)
		}

		_, err = dbConn.Exec("UPDATE subscriptions SET slug = $1 WHERE id = $2 AND slug IS NULL;", slug[:10], id)
		if err != nil {
			logrus.Fatal(err)
		}
	}
}
