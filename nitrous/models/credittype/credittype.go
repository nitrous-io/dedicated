package credittype

import (
	"database/sql"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type CreditType struct {
	ID           int64          `db:"id"`
	Name         string         `db:"name"`
	Description  sql.NullString `db:"description"`
	Active       bool           `db:"active"`
	DefaultValue int64          `db:"default_value"` // stored as cents

	Stock     sql.NullInt64 `db:"stock"`
	Used      sql.NullInt64 `db:"used"`
	ExpiresAt pq.NullTime   `db:"expires_at"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func FindValidCreditType(d db.QueryReceiver, name string) (*CreditType, error) {
	ct := &CreditType{}
	q := `SELECT * FROM credit_types
			WHERE name = $1
			AND active IS TRUE
			AND (expires_at >= now() OR expires_at IS NULL)
			AND (stock IS NULL OR used IS NULL OR used < stock);`
	err := d.Get(ct, q, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return ct, nil
}

func (i *CreditType) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO credit_types
    (name, description, active, default_value, stock, used, expires_at)
    VALUES (:name, :description, :active, :default_value, :stock, :used, :expires_at)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}
