package emailcapture

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type EmailCapture struct {
	ID        int64      `db:"id"`
	Email     string     `db:"email_address"`
	CreatedAt *time.Time `db:"created_at"`
}

func (e *EmailCapture) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO email_captures
    (email_address)
    VALUES (:email_address)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(e).StructScan(e)
}
