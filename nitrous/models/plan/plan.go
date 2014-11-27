package plan

import (
	"database/sql"
	"math"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Plan struct {
	ID            int64      `db:"id"`
	Name          string     `db:"name"`
	Slug          string     `db:"slug"`
	MaxContainers int64      `db:"max_containers"`
	Price         int64      `db:"price"`
	CPU           string     `db:"cpu"`
	Memory        string     `db:"memory"`
	Storage       string     `db:"storage"`
	Disabled      bool       `db:"disabled"`
	CreatedAt     *time.Time `db:"created_at"`
	UpdatedAt     *time.Time `db:"updated_at"`
}

func FindByID(d db.QueryReceiver, id int64) (*Plan, error) {
	plan := &Plan{}
	err := d.Get(plan, "SELECT * FROM plans WHERE id = $1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return plan, nil
}

func FindBySlug(d db.QueryReceiver, slug string) (*Plan, error) {
	plan := &Plan{}
	err := d.Get(plan, `SELECT * FROM plans WHERE slug = $1 AND disabled = $2`, slug, false)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return plan, nil
}

func All(d db.QueryReceiver) ([]*Plan, error) {
	pls := []*Plan{}
	if err := d.Select(&pls, `SELECT * FROM plans ORDER BY (price, max_containers, name)`); err != nil {
		return nil, err
	}
	return pls, nil
}

func Enabled(d db.QueryReceiver) ([]*Plan, error) {
	pls := []*Plan{}
	if err := d.Select(&pls, `SELECT * FROM plans WHERE disabled = false ORDER BY (price, max_containers, name);`); err != nil {
		return nil, err
	}
	return pls, nil
}

func (p *Plan) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO plans
		(slug, name, max_containers, price, cpu, memory, storage, disabled)
		VALUES (:slug, :name, :max_containers, :price, :cpu, :memory, :storage, :disabled)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(p).StructScan(p)
}

// Following the same pricing calculation as Digital Ocean.
// A Month will have 28 days (672 hours)
// https://www.digitalocean.com/help/pricing-and-billing/
func (p *Plan) ChargeForPeriod(t1 time.Time, t2 time.Time) uint64 {
	hours := t2.Sub(t1).Hours()
	if hours > 672.0 {
		hours = 672.0
	}
	// charge atleast for a day
	if hours < 24.0 {
		hours = 24.0
	}
	i := math.Ceil(hours * float64(p.Price) / 672.0)
	return uint64(i)
}
