package snapshot

import (
	"database/sql"
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Snapshot struct {
	ID        int64      `db:"id"`
	HostID    int64      `db:"host_id"`
	Size      int64      `db:"size"`
	Url       string     `db:"url"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

var (
	ErrSizeInvalid = errors.New("size is not valid")
	ErrUrlInvalid  = errors.New("url is not valid")
)

func Find(d db.QueryReceiver, hid int64, id int64) (*Snapshot, error) {
	v := &Snapshot{}

	err := d.Get(v, `SELECT * FROM snapshots WHERE host_id = $1 AND id = $2`, hid, id)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

func FindAll(d db.QueryReceiver, hid int64) ([]*Snapshot, error) {
	vols := []*Snapshot{}
	err := d.Select(&vols, `SELECT * FROM snapshots WHERE host_id = $1 ORDER BY created_at`, hid)
	if err != nil {
		return nil, err
	}
	return vols, nil
}

func (s *Snapshot) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO snapshots
		(host_id, size, url) VALUES
		(:host_id, :size, :url)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(s).StructScan(s)
}

func (s *Snapshot) Delete(d db.QueryReceiver) error {
	_, err := d.Exec(`DELETE FROM snapshots WHERE id = $1`, s.ID)
	return err
}

func (s *Snapshot) Validate() error {
	switch {
	case s.Size == 0:
		return ErrSizeInvalid
	case s.Url == "":
		return ErrUrlInvalid
	}
	return nil
}
