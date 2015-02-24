package invoicemanualattempt

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type InvoiceManualAttempt struct {
	ID        int64 `db:"id"`
	InvoiceID int64 `db:"invoice_id"`

	Success      bool           `db:"success"`
	ErrorMessage sql.NullString `db:"error_message"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func CreateForInvoice(d db.QueryReceiver, invoiceID int64, success bool, errMsg string) error {
	iMA := &InvoiceManualAttempt{
		InvoiceID: invoiceID,
		Success:   success,
	}
	if errMsg != "" {
		iMA.ErrorMessage = sql.NullString{errMsg, true}
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO invoice_manual_attempts
    (invoice_id, success, error_message)
    VALUES (:invoice_id, :success, :error_message)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(iMA).StructScan(iMA)
}
