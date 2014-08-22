package region

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/nitrous/db"
)

type Region struct {
	ID        int64      `db:"id"`
	Slug      string     `db:"slug"`
	Name      string     `db:"name"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func FindByID(d db.QueryReceiver, id int64) (*Region, error) {
	r := &Region{}
	err := d.Get(r, "SELECT * FROM regions WHERE id = $1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return r, nil
}

func FindBySlug(d db.QueryReceiver, slug string) (*Region, error) {
	r := &Region{}
	err := d.Get(r, "SELECT * FROM regions WHERE slug = $1", slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return r, nil
}

func All(d db.QueryReceiver) ([]*Region, error) {
	rs := []*Region{}
	if err := d.Select(&rs, `SELECT * FROM regions ORDER BY slug`); err != nil {
		return nil, err
	}
	return rs, nil
}

func (r *Region) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO regions (slug, name) VALUES (:slug, :name) RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(r).StructScan(r)
}
