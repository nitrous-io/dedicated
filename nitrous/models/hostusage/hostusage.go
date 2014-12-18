package hostusage

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type HostUsage struct {
	ID     int64 `db:"id"`
	HostID int64 `db:"host_id"`
	UserID int64 `db:"user_id"`

	Hours int        `db:"hours"`
	Day   *time.Time `db:"day"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func (h *HostUsage) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO host_usages
		(host_id, user_id, hours, day)
    VALUES (:host_id, :user_id, :hours, :day)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *HostUsage) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE host_usages
		SET (hours, updated_at)
		= (:hours, now())
		WHERE host_id = :host_id AND user_id = :user_id AND day = :day
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}
