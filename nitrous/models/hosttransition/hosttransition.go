package hosttransition

import (
	"database/sql"
	"math"
	"time"

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

type HostHourUsage struct {
	HostID  int64 `db:"host_id"`
	UserID  int64 `db:"user_id"`
	Seconds int64 `db:"seconds"`
	// HourlyPrice unit is milli-cents (x1000)
	HourlyPrice int64 `db:"hourly_price"`
}

func (h *HostHourUsage) Hours() int {
	totalHours := (time.Duration(h.Seconds) * time.Second).Hours()
	return int(math.Ceil(totalHours))
}

func FindMostRecentForHost(db *db.Database, id int64) (*HostTransition, error) {
	hT := &HostTransition{}
	err := db.Get(hT, `SELECT * FROM host_transitions WHERE host_transitions.host_id = $1 ORDER BY created_at DESC LIMIT 1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return hT, err
}

func FindByID(db *db.Database, id int64) (*HostTransition, error) {
	hT := &HostTransition{}
	err := db.Get(hT, `SELECT * FROM host_transitions WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return hT, err
}

func BillableHostHours(db *db.Database, userId int64, start, end time.Time) ([]HostHourUsage, error) {
	hostHours := []HostHourUsage{}
	err := db.Select(&hostHours,
		`SELECT host_id,
			u.id user_id,
			MAX(h.hourly_price) hourly_price,
				cast((EXTRACT(EPOCH FROM
					SUM(COALESCE(ht.ended_at, $3) - GREATEST(ht.created_at, $2)))) as integer) seconds
			FROM host_transitions ht
			JOIN hosts h ON h.id = ht.host_id
			JOIN users u ON u.id = h.user_id

			WHERE u.id = $1
				AND ht.state = 'running'
				AND (ht.created_at, COALESCE(ht.ended_at, $3)) OVERLAPS ($2, $3)
			GROUP BY host_id, u.id;`,
		userId, start, end)
	return hostHours, err
}

// FIXME: move to enforcing tx *sqlx.Tx in place of d db.QueryReceiver
func Record(d db.QueryReceiver, host *host.Host, previousState string, enactedAt time.Time) error {
	hT := &HostTransition{
		HostID:    host.ID,
		State:     host.State,
		CreatedAt: &enactedAt,
	}

	if previousState != "" {
		hT.PreviousState = sql.NullString{previousState, true}
	}

	var prevId int64
	err := d.QueryRowx(
		`SELECT id FROM host_transitions WHERE host_transitions.host_id = $1 ORDER BY created_at DESC LIMIT 1`,
		host.ID).Scan(&prevId)

	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err != sql.ErrNoRows {
		// set previous
		_, err := d.Exec(`UPDATE host_transitions SET ended_at = $1 WHERE id = $2;`, hT.CreatedAt, prevId)
		if err != nil {
			return err
		}
	}

	_, err = d.NamedExec(
		`INSERT INTO host_transitions (host_id, previous_state, state, created_at)
      VALUES (:host_id, :previous_state, :state, :created_at)`,
		hT)

	return err
}
