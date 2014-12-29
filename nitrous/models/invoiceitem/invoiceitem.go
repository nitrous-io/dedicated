package invoiceitem

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
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

	Credit int64 `db:"credit"`

	StripeID sql.NullString `db:"stripe_id"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func ItemDescription(i *InvoiceItem, hst *host.Host) string {
	if i.Credit != 0 {
		return fmt.Sprintf("Credit of $%.2f", float64(i.Credit)/100.0)
	}
	state := ""
	// FIXME: handle future states such as suspended (snapshot/resume)
	if hst.State == host.StateTerminated {
		state = " (terminated)"
	}
	prcnt := strings.TrimSuffix(fmt.Sprintf("%.3f", i.HourlyCents()), "0")
	return fmt.Sprintf("Application '%s'%s %d hours at %sc/hour", hst.Slug, state, i.Hours, prcnt)
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
	if i.Credit != 0 {
		return -1 * i.Credit
	}
	totalCents := float64(int64(i.Hours)*i.HourlyPrice) / 1000.0
	return int64(math.Ceil(totalCents))
}

func (i *InvoiceItem) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO invoice_items
    (invoice_id, host_id, hours, hourly_price, credit, stripe_id)
    VALUES (:invoice_id, :host_id, :hours, :hourly_price, :credit, :stripe_id)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}

func (i *InvoiceItem) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE invoice_items
    SET (invoice_id, host_id, hours, hourly_price, credit, stripe_id, updated_at)
    = (:invoice_id, :host_id, :hours, :hourly_price, :credit, :stripe_id, now())
    WHERE id = :id
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}
