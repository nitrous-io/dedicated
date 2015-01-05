package credittype

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type CreditType struct {
	ID          int64  `db:"id"`
	Name        string `db:"name"`
	Description string `db:"description"`
	Active      bool   `db:"active"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func (i *CreditType) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO credit_types
    (name, description, active)
    VALUES (:name, :description, :active)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}
