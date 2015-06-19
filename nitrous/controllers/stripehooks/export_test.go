package stripehooks

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
)

func SerializeInvoice(d db.QueryReceiver, inv *invoice.Invoice) (*serializers.Invoice, error) {
	return serializeInvoice(d, inv)
}

func SetSubscriptionProvider(s subscriptions.Provider) {
	subProvider = s
}
