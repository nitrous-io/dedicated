package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/go-mixpanel"
	"github.com/nitrous-io/nitrous-common/logger"
)

var (
	logr *logger.Logger
)

func init() {
	logr = &logger.Logger{Level: logger.LogLevelDebug}
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

	mixpanelToken := envvars.MixpanelToken
	if mixpanelToken == "" || strings.HasPrefix(mixpanelToken, "disable") {
		logr.FatalExit(envvars.MixpanelEnvVar, "is not defined")
	}
	mc := mixpanel.NewMixpanelClient(mixpanelToken)

	err = dbConn.MigrateAll()
	if err != nil {
		logr.Infof("Migrations failed: %v", err)
	}
	defer dbConn.DB.Close()

	users := []*user.User{}
	err = dbConn.Select(&users, "SELECT * FROM users;")
	if err != nil {
		logr.FatalExit(err)
	}

	for _, u := range users {
		logr.Infof("creating mixpanel profile for %s", u.Email)
		p := map[string]interface{}{
			"$email":    u.Email,
			"$name":     u.Name,
			"$username": u.Username,
			"$created":  u.CreatedAt,
			"user_id":   u.ID,
		}
		err := mc.CreateProfile(strconv.Itoa(int(u.ID)), p, "")
		if err != nil {
			logr.FatalExit(err)
		}
	}
}
