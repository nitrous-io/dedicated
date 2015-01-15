package credittype

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type CreditType struct {
	ID           int64  `db:"id"`
	Name         string `db:"name"`
	Description  string `db:"description"`
	Active       bool   `db:"active"`
	DefaultValue int64  `db:"default_value"` // stored as cents

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func (i *CreditType) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO credit_types
    (name, description, active, default_value)
    VALUES (:name, :description, :active, :default_value)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}
