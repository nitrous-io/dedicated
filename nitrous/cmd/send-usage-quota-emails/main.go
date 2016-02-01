package main

import (
	"database/sql"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

const (
	fiveHours = 60 * 60 * 5
	oneHour   = 60 * 60
)

func init() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})
}

func main() {
	if os.Getenv("LOG_LEVEL") != "" {
		if lvl, err := logrus.ParseLevel(os.Getenv("LOG_LEVEL")); err != nil {
			logrus.SetLevel(lvl)
		}
	}

	dbURL := envvars.NitrousPgUrl
	if dbURL == "" {
		logrus.Fatalf("%v is not defined", envvars.DBEnvVar)
	}
	dbConn, err := db.NewDatabase(dbURL)
	if err != nil {
		logrus.Fatalf("failed to connect to database, err: %v", err)
	}

	logrus.Debug("Send usage notice second warning..")
	users, err := user.FindAllByNoticeSent(dbConn, user.USAGE_NOTICES_FIRST_WARNING)
	for _, u := range users {
		seconds, err := u.FreeHostUsage(dbConn, time.Now())
		if err != nil {
			logrus.Fatalf("failed to get user's free host usage, err: %v", err)
		}
		if (u.UsageQuota.Int64 - seconds) <= oneHour {
			if err := u.SendUsageNoticeEmail(dbConn, 1); err != nil {
				logrus.Fatalf("failed to send usage notice email, err: %v", err)
			}

			u.UsageNoticesSent = sql.NullInt64{user.USAGE_NOTICES_SECOND_WARNING, true}
			if err := u.Update(dbConn); err != nil {
				logrus.Fatalf("failed to update usage_notices_sent, err: %v", err)
			}
		}
	}

	logrus.Debug("Send usage notice first warning..")
	users, err = user.FindAllByNoticeSent(dbConn, user.USAGE_NOTICES_NO_WARNING)
	for _, u := range users {
		seconds, err := u.FreeHostUsage(dbConn, time.Now())
		if err != nil {
			logrus.Fatalf("failed to get user's free host usage, err: %v", err)
		}
		if (u.UsageQuota.Int64 - seconds) <= fiveHours {
			if err := u.SendUsageNoticeEmail(dbConn, 5); err != nil {
				logrus.Fatalf("failed to send usage notice email, err: %v", err)
			}

			u.UsageNoticesSent = sql.NullInt64{user.USAGE_NOTICES_FIRST_WARNING, true}
			if err := u.Update(dbConn); err != nil {
				logrus.Fatalf("failed to update usage_notices_sent, err: %v", err)
			}
		}
	}
}
