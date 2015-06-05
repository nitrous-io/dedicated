package credit

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Credit struct {
	ID             int64         `db:"id"`
	UserID         int64         `db:"user_id"`
	InvoiceID      sql.NullInt64 `db:"invoice_id"`
	SubscriptionID sql.NullInt64 `db:"subscription_id"`
	CreditTypeID   sql.NullInt64 `db:"credit_type_id"`
	Amount         int64         `db:"amount"` // stored as cents

	AddedByID sql.NullInt64 `db:"added_by_id"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func AvailableCredits(d db.QueryReceiver, u int64) (int64, error) {
	var sum int64
	if err := d.QueryRowx(`SELECT COALESCE(SUM(amount), 0) FROM credits WHERE user_id = $1;`, u).Scan(&sum); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return sum, nil
}

func (c *Credit) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO credits
    (user_id, invoice_id, credit_type_id, amount, added_by_id)
    VALUES (:user_id, :invoice_id, :credit_type_id, :amount, :added_by_id)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}
