package biller

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
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
		UserAdmin     bool   `db:"user_admin"`
		PlanPrice     int64  `db:"plan_price"`
		PlanBasePrice int64  `db:"plan_base_price"`
		PlanStorage   string `db:"plan_storage"`
		PlanMemory    string `db:"plan_memory"`
	}
	h := &pHost{}
	err := d.Get(h, `SELECT
		hosts.*,
		users.admin user_admin,
		plans.price plan_price,
		plans.base_price plan_base_price,
		plans.storage plan_storage,
		plans.memory plan_memory
		FROM hosts
		INNER JOIN users ON users.id = hosts.user_id
		INNER JOIN plans ON plans.id = hosts.plan_id WHERE hosts.id = $1`, hostID)
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
		PlanMemory:  sql.NullString{h.PlanMemory, true},
	}

	for _, row := range rows {
		if row.State != "running" {
			continue
		}
		totalHours := (time.Duration(row.Seconds) * time.Second).Hours()
		hrs := int(math.Ceil(totalHours))
		i.Hours.Int64 += int64(hrs)
	}

	if i.Hours.Int64 == 0 {
		return 0, []invoiceitem.InvoiceItem{}, nil
	}

	if h.HourlyPrice == 0 {
		bp, err := billingprofile.Find(d, h.Host.UserID)
		if err != nil {
			return 0, []invoiceitem.InvoiceItem{}, err
		}
		if bp.State != billingprofile.StateFree && !h.UserAdmin {
			return 0, []invoiceitem.InvoiceItem{}, fmt.Errorf("Host %d has a 0 HourlyPrice but Billing profile state is incompatible", h.HourlyPrice)
		}
		if bp.MaxHours.Valid && i.Hours.Int64 > bp.MaxHours.Int64 {
			return 0, []invoiceitem.InvoiceItem{}, fmt.Errorf("Host %d exceeds the maximum hours of the billing profile (%d vs %d)", h.HourlyPrice, i.Hours.Int64, bp.MaxHours.Int64)
		}
		i.ItemType = invoiceitem.ItemTypeFreeCompute
	}

	applyRulesToInvoiceItem(&i, l)
	items := []invoiceitem.InvoiceItem{i}
	totalCharge := i.TotalCharge(&plan.Plan{Price: h.PlanPrice})

	// apply base workspace price
	if h.PlanBasePrice > 0 && (h.TerminatedAt == nil || h.TerminatedAt.After(start)) {
		startingPoint := start
		if h.CreatedAt.After(startingPoint) {
			startingPoint = *h.CreatedAt
		}
		endPoint := end
		if h.TerminatedAt != nil {
			endPoint = *h.TerminatedAt
		}

		totalHours := int64(math.Ceil(float64(endPoint.Sub(startingPoint)/time.Second) / 60.0 / 60.0))
		hourlyPlanPrice := int64(math.Ceil(float64(h.PlanBasePrice) / invoiceitem.MaxHours * 1000))
		ebsI := invoiceitem.InvoiceItem{
			Hours:       sql.NullInt64{totalHours, true},
			HourlyPrice: sql.NullInt64{hourlyPlanPrice, true},
			HostID:      sql.NullInt64{hostID, true},
			Terminated:  h.TerminatedAt != nil,
			ItemType:    invoiceitem.ItemTypeBaseEBS,
			PlanStorage: sql.NullString{h.PlanStorage, true},
		}
		applyRulesToInvoiceItem(&ebsI, l)
		totalCharge += ebsI.TotalCharge(&plan.Plan{BasePrice: h.PlanBasePrice})
		items = append(items, ebsI)
	}

	return totalCharge, items, nil
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
