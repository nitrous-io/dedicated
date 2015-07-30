package invoice

import (
	"database/sql"
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
)

var (
	Mailer                 mailer.Mailer
	ErrNegativeTotalCharge = errors.New("Negative TotalCharge")
	ErrEmptyState          = errors.New("state must be provided")
)

const (
	StateUnpaid     = "unpaid"
	StatePaid       = "paid"
	StateFailed     = "failed"
	StateDeferred   = "deferred"
	StateNotPayable = "not_payable"
)

func init() {
	Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)
}

type Invoice struct {
	ID     int64 `db:"id"`
	UserID int64 `db:"user_id"`

	SubscriptionID sql.NullInt64 `db:"subscription_id"`

	// TotalCharge is measured in cents (unlike HourlyPrice it derives from)
	TotalCharge     int64         `db:"total_charge"`
	CreditAmount    int64         `db:"credit_amount"` // how much was taken off by credits
	AvailableCredit sql.NullInt64 `db:"available_credit"`

	PeriodStart *time.Time `db:"period_start"`
	PeriodEnd   *time.Time `db:"period_end"`

	// TODO: Remove after satisfied with migration
	NSuccessfulCharge bool `db:"successful_charge"`
	NDeferred         bool `db:"deferred"`
	NFailed           bool `db:"failed"`

	// current state of invoice
	State string `db:"state"`

	// indicates another invoice included the total for this invoice
	CoveredBy sql.NullInt64 `db:"covered_by"`

	FirstAttemptAt *time.Time     `db:"first_attempt_at"`
	LastAttemptAt  *time.Time     `db:"last_attempt_at"`
	Attempts       int            `db:"attempts"`
	StripeID       sql.NullString `db:"stripe_id"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func ForPeriod(d db.QueryReceiver, userID int64, periodStart, periodEnd time.Time) (*Invoice, error) {
	inv := &Invoice{}
	err := d.Get(inv, `SELECT * FROM invoices WHERE user_id = $1 AND period_start = $2 AND period_end = $3`,
		userID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}
	return inv, err
}

func Find(d db.QueryReceiver, id int64) (*Invoice, error) {
	inv := &Invoice{}
	if err := d.Get(inv, `SELECT * FROM invoices where id = $1;`, id); err != nil {
		return nil, err
	}
	return inv, nil
}

func FindByStripeID(d db.QueryReceiver, stripeID string) (*Invoice, error) {
	inv := &Invoice{}
	if err := d.Get(inv, `SELECT * FROM invoices WHERE stripe_id = $1`, stripeID); err != nil {
		return nil, err
	}
	return inv, nil
}

func (i *Invoice) Insert(d db.QueryReceiver) error {
	if err := i.validate(); err != nil {
		return err
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO invoices
    (user_id, subscription_id, total_charge, credit_amount, period_start, period_end, state,
      available_credit, first_attempt_at, last_attempt_at, attempts, stripe_id, covered_by)
    VALUES (:user_id, :subscription_id, :total_charge, :credit_amount, :period_start, :period_end, :state,
      :available_credit, :first_attempt_at, :last_attempt_at, :attempts, :stripe_id, :covered_by)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}

func (i *Invoice) Update(d db.QueryReceiver) error {
	if err := i.validate(); err != nil {
		return err
	}
	nstmt, err := d.PrepareNamed(`UPDATE invoices
    SET (user_id, subscription_id, total_charge, credit_amount, period_start, period_end, state,
      available_credit, first_attempt_at, last_attempt_at, attempts, stripe_id, covered_by, updated_at)
    = (:user_id, :subscription_id, :total_charge, :credit_amount, :period_start, :period_end, :state,
      :available_credit, :first_attempt_at, :last_attempt_at, :attempts, :stripe_id, :covered_by, now())
    WHERE id = :id
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}

func (i *Invoice) Delete(d db.QueryReceiver) error {
	_, err := d.Exec(`DELETE FROM invoices WHERE id = $1`, i.ID)
	return err
}

func (i *Invoice) validate() error {
	if i.State == "" {
		return ErrEmptyState
	}
	if i.TotalCharge < 0 {
		return ErrNegativeTotalCharge
	}
	return nil
}

func (i *Invoice) PaymentSucceeded(d *db.Database) error {
	i.State = StatePaid
	t := time.Now()
	if i.FirstAttemptAt == nil || i.FirstAttemptAt.IsZero() {
		i.FirstAttemptAt = &t
	}
	i.LastAttemptAt = &t
	if err := i.Update(d); err != nil {
		return err
	}
	_, err := d.Exec("UPDATE invoices SET state = $1, last_attempt_at = now() WHERE covered_by = $2;", StatePaid, i.ID)
	if err != nil {
		return err
	}

	// reflect state of billingprofile
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE billing_profiles
		SET state = $1
		WHERE user_id = $2
                AND org_id IS NULL
		AND state NOT IN ($3, $4)
		AND NOT EXISTS (
			SELECT id FROM invoices
				WHERE user_id = $2
				AND state NOT IN ($5, $6)
				AND total_charge > 0 -- invoices that are fully covered by credits have a 0 total_charge
			FOR UPDATE)`,
		billingprofile.StatePaid, i.UserID, billingprofile.StateFree, billingprofile.StateExempt, StateNotPayable, StatePaid)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
