package stripehooks

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
	"github.com/nitrous-io/nitrous-common/logger"
)

func SerializeInvoice(d db.QueryReceiver, inv *invoice.Invoice, logger *logger.Logger) (*serializers.Invoice, error) {
	return serializeInvoice(d, inv, logger)
}

func SetSubscriptionProvider(s subscriptions.Provider) {
	subProvider = s
}
