package main

import (
	"database/sql"
	"errors"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
)

type HostTransition struct {
	ID          int64         `db:"id"`
	HostID      int64         `db:"host_id"`
	InitiatorID sql.NullInt64 `db:"initiator_id"`

	PreviousState sql.NullString `db:"previous_state"`
	State         string         `db:"state"`

	CreatedAt *time.Time `db:"created_at"`
	EndedAt   *time.Time `db:"ended_at"`

	Metadata hstore.Hstore `db:"metadata"`
}

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

	rows, err := dbConn.Query("SELECT id FROM hosts")
	if err != nil {
		logrus.Fatalf("failed to query all hosts, err: %v", err)
	}

	for rows.Next() {
		dbConn.WithTransaction(func(tx *sqlx.Tx) error {
			var hostID int
			err = rows.Scan(&hostID)
			if err != nil {
				logrus.Fatalf("failed to scan host id, err: %v", err)
				return err
			}

			nullEndedAtHostTransitions := []*HostTransition{}
			// select all running host transitions with empty ended_at
			// and omit last row.
			err = tx.Select(&nullEndedAtHostTransitions, `
				SELECT * FROM host_transitions
				WHERE host_id = $1
				AND ended_at IS NULL
				AND state = 'running'
				AND id < (
					SELECT MAX(id) FROM host_transitions WHERE host_id = $1
				)
				ORDER BY created_at
			`, hostID)
			if err != nil {
				logrus.Fatalf("failed to get nullEndedAtHostTransitions, err: %v", err)
				if err == sql.ErrNoRows {
					logrus.Debugf("No problem host transitions found for host %d", hostID)
				}
				return err
			}

			for _, ht := range nullEndedAtHostTransitions {
				nextOfNullEndedAtHostTransitions := &HostTransition{}
				err = tx.Get(nextOfNullEndedAtHostTransitions, `
					SELECT * FROM host_transitions
					WHERE host_id = $1
					AND created_at > $2
					ORDER BY created_at
					LIMIT 1
				`, hostID, ht.CreatedAt)
				if err != nil {
					logrus.Fatalf("failed to get nextOfNullEndedAtHostTransitions, err: %v", err)
					return err
				}
				if nextOfNullEndedAtHostTransitions.CreatedAt == nil {
					logrus.Debugf("created_at of next host_transitions is nil for host %d", hostID)
					return errors.New("created_at can't be blank")
				}

				newEndedAt := nextOfNullEndedAtHostTransitions.CreatedAt
				_, err = tx.Exec(`
					UPDATE host_transitions SET ended_at = $1 WHERE id = $2
				`, newEndedAt, ht.ID)
				if err != nil {
					logrus.Fatalf("failed to update host transition, err: %v", err)
					return err
				}

				logrus.Debugf("Updated host transition id: %d, ended_at: %v", ht.ID, newEndedAt)
			}
			return nil
		})
	}
}
