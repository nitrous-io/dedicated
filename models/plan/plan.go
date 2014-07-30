package plan

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/nitrous/db"
)

type Plan struct {
	ID            int64      `db:"id"`
	Name          string     `db:"name"`
	MaxContainers int64      `db:"max_containers"`
	Price         int64      `db:"price"`
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

func FindByName(d db.QueryReceiver, name string) (*Plan, error) {
	plan := &Plan{}
	err := d.Get(plan, `SELECT * FROM plans WHERE name = $1`, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return plan, nil
}

func (p *Plan) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO plans
		(name, max_containers, price)
		VALUES (:name, :max_containers, :price)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(p).StructScan(p)
}
