package biller

import (
	"database/sql"
	"math"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/nitrous-common/logger"
)

type InvoiceItemsResult struct {
	TotalCharge int64
	Items       []invoiceitem.InvoiceItem
}

func InvoiceItemsForUser(d db.QueryReceiver, l *logger.Logger, userID int64, start, end time.Time) (InvoiceItemsResult, error) {
	res := InvoiceItemsResult{}

	var ids []int64
	err := d.Select(&ids, `SELECT id FROM hosts WHERE user_id = $1 AND (terminated_at IS NULL OR terminated_at >= $2);`, userID, start)
	if err != nil {
		return res, err
	}

	for _, id := range ids {
		charge, hItems, err := invoicesItemsForHost(d, l, id, start, end)
		if err != nil {
			return res, err
		}
		res.TotalCharge += charge
		for _, item := range hItems {
			res.Items = append(res.Items, item)
		}
	}

	return res, nil
}

// returns a collection of invoice items for a host
// currently a single invoice item is returned,
// but a collection may be returned in future iterations of the biller
func invoicesItemsForHost(d db.QueryReceiver, l *logger.Logger, hostID int64, start, end time.Time) (int64, []invoiceitem.InvoiceItem, error) {
	type pHost struct {
		host.Host
		PlanPrice int64 `db:"plan_price"`
	}
	h := &pHost{}
	err := d.Get(h, "SELECT hosts.*, plans.price plan_price FROM hosts INNER JOIN plans ON plans.id = hosts.plan_id WHERE hosts.id = $1", hostID)
	if err != nil {
		return 0, []invoiceitem.InvoiceItem{}, err
	}

	type row struct {
		State   string `db:"state"`
		Seconds int64  `db:"seconds"`
	}
	rows := []row{}
	err = d.Select(&rows,
		`SELECT
      cast((EXTRACT(EPOCH FROM
        SUM(COALESCE(ht.ended_at, $3) - GREATEST(ht.created_at, $2)))) as integer) seconds,
      ht.state
      FROM host_transitions ht

      WHERE ht.host_id = $1
        AND (ht.created_at, COALESCE(ht.ended_at, $3)) OVERLAPS ($2, $3)
      GROUP BY state;`,
		hostID, start, end)
	if err != nil && err != sql.ErrNoRows {
		return 0, []invoiceitem.InvoiceItem{}, err
	}
	if len(rows) == 0 {
		return 0, []invoiceitem.InvoiceItem{}, nil
	}

	i := invoiceitem.InvoiceItem{
		Hours:       sql.NullInt64{0, true},
		HourlyPrice: sql.NullInt64{h.HourlyPrice, true},
		HostID:      sql.NullInt64{hostID, true},
		Terminated:  h.TerminatedAt != nil,
		ItemType:    invoiceitem.ItemTypeCompute,
	}

	for _, row := range rows {
		if row.State != "running" {
			continue
		}
		totalHours := (time.Duration(row.Seconds) * time.Second).Hours()
		hrs := int(math.Ceil(totalHours))
		i.Hours.Int64 += int64(hrs)
	}

	applyRulesToInvoiceItem(&i, l)

	return i.TotalCharge(&plan.Plan{Price: h.PlanPrice}), []invoiceitem.InvoiceItem{i}, nil
}

func applyRulesToInvoiceItem(itm *invoiceitem.InvoiceItem, l *logger.Logger) {
	// TODO: pull rules from database
	if itm.Hours.Int64 > invoiceitem.MaxHours {
		origHrs := itm.Hours.Int64
		itm.Hours.Int64 = invoiceitem.MaxHours
		if l != nil {
			l.Infof("Invoice for host %d, capping to %d hrs. original = %d hrs", itm.HostID.Int64, itm.Hours.Int64, origHrs)
		}
	}
}
