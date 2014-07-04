package region

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/nitrous/db"
)

type Region struct {
	ID          int64      `db:"id"`
	Name        string     `db:"name"`
	Description string     `db:"description"`
	CreatedAt   *time.Time `db:"created_at"`
	UpdatedAt   *time.Time `db:"updated_at"`
}

func FindByName(d db.QueryReceiver, name string) (*Region, error) {
	r := &Region{}
	err := d.Get(r, "SELECT * FROM regions WHERE name = $1", name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return r, nil
}

func (r *Region) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO regions (name, description) VALUES (:name, :description) RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(r).StructScan(r)
}
