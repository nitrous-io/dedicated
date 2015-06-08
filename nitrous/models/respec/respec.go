package respec

import (
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

const (
	StatePending    = "pending"
	StateProcessing = "processing"
	StateCompleted  = "completed"
	StateFailed     = "failed"
	StateObsolete   = "obsolete" // A respec becomes obsolete when there's a newer respec.
)

type Respec struct {
	ID                 int64 `db:"id"`
	HostID             int64 `db:"host_id"`
	ProviderTemplateID int64 `db:"provider_template_id"`

	State string `db:"state"`

	CreatedAt pq.NullTime `db:"created_at"`
	UpdatedAt pq.NullTime `db:"updated_at"`
}

// Find returns the pending respec for a host. There can only be 1 pending
// respec for a host.
func Find(d *db.Database, hostID int64) (*Respec, error) {
	r := &Respec{}
	err := d.Get(r, `SELECT * FROM respecs
		WHERE host_id = $1 AND state = $2
		ORDER BY created_at LIMIT 1`, hostID, StatePending)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Respec) Insert(tx *sqlx.Tx) error {
	if r.HostID == 0 {
		return errors.New("HostID is required")
	}

	// Mark all of the host's pending respecs as obsolete before inserting this
	// new respec.
	_, err := tx.Exec(`UPDATE respecs SET state = $1
		WHERE host_id = $2 AND state = $3`, StateObsolete, r.HostID, StatePending)
	if err != nil {
		return err
	}

	r.State = StatePending
	nstmt, err := tx.PrepareNamed(`INSERT INTO respecs
		(host_id, provider_template_id, state)
		VALUES
		(:host_id, :provider_template_id, :state)
		RETURNING *`)
	if err != nil {
		return err
	}

	return nstmt.QueryRowx(r).StructScan(r)
}

func (r *Respec) UpdateState(d *db.Database, state string) error {
	r.State = state
	nstmt, err := d.PrepareNamed(`UPDATE respecs
		SET (state, updated_at) = (:state, now())
		WHERE id = :id RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(r).StructScan(r)
}
