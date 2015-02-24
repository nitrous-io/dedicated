package stripehooks

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func SerializeInvoice(d db.QueryReceiver, inv *invoice.Invoice) (*serializers.Invoice, error) {
	return serializeInvoice(d, inv)
}
