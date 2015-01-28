package credittype

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

var (
	SignupCreditType = "Signup_Credit"
)

type CreditType struct {
	ID           int64          `db:"id"`
	Name         string         `db:"name"`
	Description  sql.NullString `db:"description"`
	Active       bool           `db:"active"`
	DefaultValue int64          `db:"default_value"` // stored as cents

	Stock sql.NullInt64 `db:"stock"`
	Used  sql.NullInt64 `db:"used"`

	ExpiresAt *time.Time `db:"expires_at"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func FindValidCreditTypeByName(d db.QueryReceiver, name string) (*CreditType, error) {
	c := &CreditType{}
	err := d.Get(c, `
		SELECT * FROM credit_types
		WHERE name = $1
		AND active = $2
		AND (expires_at IS NULL OR expires_at >= now())
		AND (stock IS NULL OR used IS NULL OR used < stock);
	`, name, true)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func (c *CreditType) Insert(d db.QueryReceiver) error {
	var nstmt *sqlx.NamedStmt
	var err error

	// so that we can avoid NullTime/zero time issue
	if c.ExpiresAt != nil {
		nstmt, err = d.PrepareNamed(`INSERT INTO credit_types
			(name, description, active, default_value, stock, used, expires_at)
			VALUES (:name, :description, :active, :default_value, :stock, :used, :expires_at)
			RETURNING *;`)
	} else {
		nstmt, err = d.PrepareNamed(`INSERT INTO credit_types
			(name, description, active, default_value, stock, used)
			VALUES (:name, :description, :active, :default_value, :stock, :used)
			RETURNING *;`)
	}

	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}
