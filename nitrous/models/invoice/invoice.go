package invoice

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Invoice struct {
	ID     int64 `db:"id"`
	UserID int64 `db:"user_id"`

	// TotalCharge is measured in cents (unlike HourlyPrice it derives from)
	TotalCharge int64 `db:"total_charge"`

	PeriodStart *time.Time `db:"period_start"`
	PeriodEnd   *time.Time `db:"period_end"`

	SuccessfulCharge bool           `db:"successful_charge"`
	FirstAttemptAt   *time.Time     `db:"first_attempt_at"`
	LastAttemptAt    *time.Time     `db:"last_attempt_at"`
	Attempts         int            `db:"attempts"`
	StripeID         sql.NullString `db:"stripe_id"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func ForPeriod(d db.QueryReceiver, userID int64, periodStart, periodEnd time.Time) (*Invoice, error) {
	inv := &Invoice{}
	err := d.Get(inv, `SELECT * FROM invoices WHERE user_id = $1 AND period_start = $2 AND period_end = $3`,
		userID, periodStart, periodEnd)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return inv, err
}

func (i *Invoice) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO invoices
    (user_id, total_charge, period_start, period_end, successful_charge,
      first_attempt_at, last_attempt_at, attempts, stripe_id)
    VALUES (:user_id, :total_charge, :period_start, :period_end, :successful_charge,
      :first_attempt_at, :last_attempt_at, :attempts, :stripe_id)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}

func (i *Invoice) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE invoices
    SET (user_id, total_charge, period_start, period_end, successful_charge,
      first_attempt_at, last_attempt_at, attempts, stripe_id, updated_at)
    = (:user_id, :total_charge, :period_start, :period_end, :successful_charge,
      :first_attempt_at, :last_attempt_at, :attempts, :stripe_id, now())
    WHERE id = :id
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}
