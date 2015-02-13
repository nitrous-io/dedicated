package region

import (
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Region struct {
	ID         int64       `db:"id"`
	Slug       string      `db:"slug"`
	Name       string      `db:"name"`
	Rank       int64       `db:"rank"`
	DisabledAt pq.NullTime `db:"disabled_at"`
	CreatedAt  *time.Time  `db:"created_at"`
	UpdatedAt  *time.Time  `db:"updated_at"`
}

var (
	ErrRegionDisabled = errors.New("Region is disabled")
)

func FindByID(d db.QueryReceiver, id int64) (*Region, error) {
	r := &Region{}
	err := d.Get(r, "SELECT * FROM regions WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func FindBySlug(d db.QueryReceiver, slug string) (*Region, error) {
	r := &Region{}
	err := d.Get(r, "SELECT * FROM regions WHERE slug = $1", slug)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func All(d db.QueryReceiver) ([]*Region, error) {
	rs := []*Region{}
	if err := d.Select(&rs, `SELECT * FROM regions WHERE disabled_at IS NULL OR disabled_at > now() ORDER BY rank`); err != nil {
		return nil, err
	}
	return rs, nil
}

func (r *Region) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO regions (slug, name, rank) VALUES (:slug, :name, :rank) RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(r).StructScan(r)
}

func (r *Region) Disabled() bool {
	if !r.DisabledAt.Valid {
		return false
	}
	return !r.DisabledAt.Time.After(time.Now())
}
