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
	nstmt, err := d.PrepareNamed(`INSERT INTO regions (name, description) VALUES (:name, :description) RETURNING id;`)
	if err != nil {
		return err
	}
	var id int64
	err = nstmt.QueryRow(r).Scan(&id)
	if err != nil {
		return err
	}
	return d.Get(r, "SELECT * FROM regions WHERE id = $1;", id)
}
