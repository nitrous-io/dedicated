package biller

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
)

func (b *Biller) CreateOrUpdateInvoice() (*InvoiceResult, error) {
	return b.createOrUpdateInvoice()
}

func (b *Biller) ProcessStripe(i *InvoiceResult) error {
	return b.processStripe(i)
}

func InvoicesItemsForHost(d db.QueryReceiver, hostID int64, start, end time.Time) (int64, []invoiceitem.InvoiceItem, error) {
	return invoicesItemsForHost(d, hostID, start, end)
}
