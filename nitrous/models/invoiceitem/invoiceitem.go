package invoiceitem

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

type InvoiceItem struct {
	ID        int64 `db:"id"`
	InvoiceID int64 `db:"invoice_id"`
	HostID    int64 `db:"host_id"`

	Hours int `db:"hours"`
	// HourlyPrice unit is milli-cents (x1000)
	HourlyPrice int64 `db:"hourly_price"`

	StripeID sql.NullString `db:"stripe_id"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func ItemDescription(i *InvoiceItem, hst *host.Host) string {
	state := ""
	// FIXME: handle future states such as suspended (snapshot/resume)
	if hst.State == host.StateTerminated {
		state = " (terminated)"
	}
	return fmt.Sprintf("Application '%s'%s %d hours at %.2fc/hour", hst.Slug, state, i.Hours, i.HourlyCents())
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
	return float64(i.HourlyPrice) / float64(1000)
}

func (i *InvoiceItem) TotalCharge() int64 {
	return (int64(i.Hours) * i.HourlyPrice) / 1000
}

func (i *InvoiceItem) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO invoice_items
    (invoice_id, host_id, hours, hourly_price, stripe_id)
    VALUES (:invoice_id, :host_id, :hours, :hourly_price, :stripe_id)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}

func (i *InvoiceItem) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE invoice_items
    SET (invoice_id, host_id, hours, hourly_price, stripe_id, updated_at)
    = (:invoice_id, :host_id, :hours, :hourly_price, :stripe_id, now())
    WHERE id = :id
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}
