package biller

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
)

func InvoicesItemsForHost(d db.QueryReceiver, hostID int64, start, end time.Time) (int64, []invoiceitem.InvoiceItem, error) {
	return invoicesItemsForHost(d, hostID, start, end)
}
