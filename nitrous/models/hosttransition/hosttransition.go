package hosttransition

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

type HostTransition struct {
	ID     int64 `db:"id"`
	HostID int64 `db:"host_id"`

	PreviousState sql.NullString `db:"previous_state"`
	State         string         `db:"state"`

	CreatedAt *time.Time `db:"created_at"`
	EndedAt   *time.Time `db:"ended_at"`
}

func FindMostRecentForHost(db *db.Database, id int64) (*HostTransition, error) {
	hT := &HostTransition{}
	err := db.Get(hT, `SELECT * FROM host_transitions WHERE host_transitions.host_id = $1 ORDER BY created_at DESC LIMIT 1`, id)
	if err != nil {
		return nil, err
	}
	return hT, err
}

func FindByID(db *db.Database, id int64) (*HostTransition, error) {
	hT := &HostTransition{}
	err := db.Get(hT, `SELECT * FROM host_transitions WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return hT, err
}

func Record(tx *sqlx.Tx, host *host.Host, previousState string, enactedAt time.Time) error {
	hT := &HostTransition{
		HostID:    host.ID,
		State:     host.State,
		CreatedAt: &enactedAt,
	}

	if previousState != "" {
		hT.PreviousState = sql.NullString{previousState, true}
	}

	var prevId int64
	err := tx.QueryRowx(
		`SELECT id FROM host_transitions WHERE host_transitions.host_id = $1 ORDER BY created_at DESC LIMIT 1`,
		host.ID).Scan(&prevId)

	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err != sql.ErrNoRows {
		// set previous
		_, err := tx.Exec(`UPDATE host_transitions SET ended_at = $1 WHERE id = $2;`, hT.CreatedAt, prevId)
		if err != nil {
			return err
		}
	}

	_, err = tx.NamedExec(
		`INSERT INTO host_transitions (host_id, previous_state, state, created_at)
      VALUES (:host_id, :previous_state, :state, :created_at)`,
		hT)

	return err
}
