package stripehooks

import (
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/nitrous-common/logger"
	stripe "github.com/stripe/stripe-go"
	stripeinvoice "github.com/stripe/stripe-go/invoice"
)

var (
	HandlePaymentSucceededFn func(*middleware.Env, *invoice.Invoice)            = handlePaymentSucceeded
	HandlePaymentFailedFn    func(*middleware.Env, *invoice.Invoice, int, bool) = handlePaymentFailed
)

func handleStripeInvoiceEvent(env *middleware.Env, e *stripe.Event, stripeID string) {
	inv, err := invoice.FindByStripeID(env.DB, stripeID)
	if err != nil {
		env.Logger.Errorf("Error when locating invoice %s, err: %v", stripeID, err)
		return
	}
	if inv == nil {
		env.Logger.Errorf("Could not find invoice matching Stripe event: %+v", e)
		return
	}

	// webhook data is found to be outdated, query the invoice for *live* info
	strInv, err := stripeinvoice.Get(stripeID, nil)
	if err != nil {
		env.Logger.Errorf("Error requesting Stripe for invoice %s, err: %+v", stripeID, err)
		return
	}

	if strInv.Paid {
		HandlePaymentSucceededFn(env, inv)
	} else {
		HandlePaymentFailedFn(env, inv, int(strInv.Attempts), strInv.NextAttempt == 0)
	}
	u, err := user.FindByID(env.DB, inv.UserID)
	if err != nil {
		env.Logger.Errorf("Error looking up user %d for invoice %d, err: %+v", inv.UserID, inv.ID, err)
		return
	}
	sInv, err := serializeInvoice(env.DB, inv, env.Logger)
	if err != nil {
		env.Logger.Errorf("Error serializing invoice %d for user %d, err: %+v", inv.UserID, inv.ID, err)
		return
	}
	env.Events.Publish(
		controllers.NewEvent(controllers.EventInvoiceUpdated, map[string]interface{}{
			"invoice": sInv,
		}),
		u.Topic(),
	)
}

func serializeInvoice(d db.QueryReceiver, inv *invoice.Invoice, logger *logger.Logger) (*serializers.Invoice, error) {
	sInv := serializers.SerializeInvoice(inv)

	items := []invoiceitem.InvoiceItem{}
	err := d.Select(&items, "SELECT * FROM invoice_items WHERE invoice_id = $1 ORDER BY id DESC;", inv.ID)
	if err != nil {
		return nil, err
	}

	type hst struct {
		HostID    int64  `db:"host_id"`
		HostSlug  string `db:"host_slug"`
		PlanPrice int64  `db:"plan_price"`
	}
	hosts := []hst{}
	err = d.Select(&hosts, "SELECT hosts.id host_id, hosts.slug host_slug, plans.price plan_price FROM hosts INNER JOIN invoice_items ON invoice_items.host_id = hosts.id INNER JOIN plans ON hosts.plan_id = plans.id WHERE invoice_items.invoice_id = $1 ORDER BY hosts.id DESC;", inv.ID)
	if err != nil {
		return nil, err
	}
	hMap := make(map[int64]hst)
	for indx, host := range hosts {
		hMap[host.HostID] = hosts[indx]
	}

	for _, item := range items {
		if item.Credit.Valid {
			sInv.TotalCredit += item.Credit.Int64
		} else {
			ser := serializers.SerializeInvoiceItem(&item)
			if hst, ok := hMap[ser.HostID]; ok {
				ser.HostSlug = hst.HostSlug
				ser.Description = invoiceitem.ItemDescription(&item, &host.Host{Slug: hst.HostSlug})
				ser.TotalCharge = item.TotalCharge(&plan.Plan{Price: hst.PlanPrice})
			} else {
				ser.Description = "Information not available at this time"
				ser.TotalCharge = item.UncappedTotalCharge()
				if logger != nil {
					logger.Errorf("stripehooks: Unable to locate host %d in hMap for user %d", ser.HostID, inv.UserID)
				}
			}
			sInv.Items = append(sInv.Items, *ser)
		}
	}
	return sInv, nil
}

func handlePaymentSucceeded(env *middleware.Env, inv *invoice.Invoice) {
	if err := inv.PaymentSucceeded(env.DB); err != nil {
		env.Logger.Errorf("Error marking invoice %d as succeeded, err: %v", inv.ID, err)
	}
}

func handlePaymentFailed(env *middleware.Env, inv *invoice.Invoice, attemptCount int, attemptsExhausted bool) {
	if err := inv.PaymentFailed(env.DB, attemptCount, attemptsExhausted); err != nil {
		env.Logger.Errorf("Error marking invoice %d as failed, err: %v", inv.ID, err)
	}
}
