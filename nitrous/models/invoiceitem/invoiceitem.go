package invoiceitem

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

type InvoiceItem struct {
	ID        int64         `db:"id"`
	InvoiceID int64         `db:"invoice_id"`
	HostID    sql.NullInt64 `db:"host_id"`

	Hours sql.NullInt64 `db:"hours"`
	// HourlyPrice unit is milli-cents (x1000)
	HourlyPrice sql.NullInt64 `db:"hourly_price"`
	Terminated  bool          `db:"terminated"`

	Credit sql.NullInt64 `db:"credit"`

	StripeID sql.NullString `db:"stripe_id"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

const MaxHours = 28 * 24

var (
	ErrMissingHostIDCredit = errors.New("Host ID or Credit value != 0 must be provided")
	ErrCreditInvalid       = errors.New("Credit value must be > 0")
	ErrHoursInvalid        = errors.New("Hours must not be NULL")
	ErrHourlyPriceInvalid  = errors.New("Hourly Price must not be NULL")
	ErrHoursNotPositive    = errors.New("Hours must be greater than 0")
	ErrHourlyPriceNegative = errors.New("HourlyPrice must not be a negative")
	ErrHoursExceedMaxHours = errors.New("Hours must be less than/equal to MaxHours")
)

func ItemDescription(i *InvoiceItem, hst *host.Host) string {
	if i.Credit.Valid && i.Credit.Int64 != 0 {
		return fmt.Sprintf("Credit of $%.2f", float64(i.Credit.Int64)/100.0)
	}
	state := ""
	// FIXME: handle future states such as suspended (snapshot/resume)
	if hst.State == host.StateTerminated {
		state = " (terminated)"
	}
	prcnt := strings.TrimSuffix(fmt.Sprintf("%.3f", i.HourlyCents()), "0")
	return fmt.Sprintf("Application '%s'%s %d hours at %sc/hour", hst.Slug, state, i.Hours.Int64, prcnt)
}

func FindAllForInvoice(d db.QueryReceiver, iID int64) ([]InvoiceItem, error) {
	items := []InvoiceItem{}
	err := d.Select(&items, "SELECT * FROM invoice_items WHERE invoice_id = $1 ORDER BY id;", iID)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (i *InvoiceItem) HourlyCents() float64 {
	return float64(i.HourlyPrice.Int64) / float64(1000)
}

func (i *InvoiceItem) TotalCharge() int64 {
	if i.Credit.Valid && i.Credit.Int64 != 0 {
		return -1 * i.Credit.Int64
	}
	totalCents := float64(i.Hours.Int64*i.HourlyPrice.Int64) / 1000.0
	return int64(math.Ceil(totalCents))
}

func (i *InvoiceItem) Insert(d db.QueryReceiver) error {
	if err := i.validate(); err != nil {
		return err
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO invoice_items
    (invoice_id, host_id, terminated, hours, hourly_price, credit, stripe_id)
    VALUES (:invoice_id, :host_id, :terminated, :hours, :hourly_price, :credit, :stripe_id)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}

func (i *InvoiceItem) Update(d db.QueryReceiver) error {
	if err := i.validate(); err != nil {
		return err
	}
	nstmt, err := d.PrepareNamed(`UPDATE invoice_items
    SET (invoice_id, host_id, hours, terminated, hourly_price, credit, stripe_id, updated_at)
    = (:invoice_id, :host_id, :hours, :terminated, :hourly_price, :credit, :stripe_id, now())
    WHERE id = :id
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}

func (i *InvoiceItem) validate() error {
	if !i.HostID.Valid && !i.Credit.Valid {
		return ErrMissingHostIDCredit
	}
	if !i.HostID.Valid && i.Credit.Int64 <= 0 {
		return ErrCreditInvalid
	}
	if i.HostID.Valid {
		if !i.Hours.Valid {
			return ErrHoursInvalid
		}
		if i.Hours.Int64 <= 0 {
			return ErrHoursNotPositive
		}
		if i.Hours.Int64 > MaxHours {
			return ErrHoursExceedMaxHours
		}
		if !i.HourlyPrice.Valid {
			return ErrHourlyPriceInvalid
		}
		if i.HourlyPrice.Int64 < 0 {
			return ErrHourlyPriceNegative
		}
	}
	return nil
}
