package main

import (
	"database/sql"
	"os"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/nitrous-common/logger"
)

const DBEnvVar = "NITROUS_PG_URL"

var (
	logr        *logger.Logger
	initiatorID int64
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

	if envvars.ProvisionerUser == "" {
		logr.FatalExit("PROVISIONER_USER is not defined")
	}

	err = dbConn.QueryRow(`SELECT id FROM users WHERE username = $1;`, envvars.ProvisionerUser).Scan(&initiatorID)
	if err != nil {
		logr.FatalfExit("Failed to locate the ID of the PROVISIONER_USER. Err: '%s'", err)
	}

	lastId := int64(-1)
	hostsExhausted := false

	// page through the hosts in the DB, 100 at a time
	for !hostsExhausted {
		hosts := []host.Host{}
		err := dbConn.Select(&hosts, `SELECT * FROM hosts h WHERE NOT EXISTS (SELECT t.id FROM host_transitions t WHERE t.host_id = h.id)
			AND h.id > $1 AND h.preprovisioned = FALSE ORDER BY id ASC LIMIT 100;`, lastId)
		if err != nil && err != sql.ErrNoRows {
			logr.FatalExit(err)
		}

		if err == sql.ErrNoRows {
			hostsExhausted = true
			break
		}

		if len(hosts) == 0 {
			hostsExhausted = true
			break
		}

		for _, host := range hosts {
			if host.ID > lastId {
				lastId = host.ID
			}
			time := host.AssignedAt
			if time == nil {
				time = host.CreatedAt
			}

			logr.Infof("Creating host transition '%s' for %s", host.State, host.Slug)
			tx, err := dbConn.Beginx()
			if err != nil {
				logr.Infof("Failed starting transaction for for %d '%s'", host.ID, host.Slug)
				continue
			}
			if err := hosttransition.Record(tx, &host, "", *time, initiatorID); err != nil {
				tx.Rollback()
				logr.Infof("Failed to populate host transition for %d '%s'", host.ID, host.Slug)
				continue
			}
			if err := tx.Commit(); err != nil {
				logr.Infof("Failed to commit host transition for %d '%s'", host.ID, host.Slug)
			}
		}
	}
}
