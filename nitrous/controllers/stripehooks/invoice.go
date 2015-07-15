package stripehooks

import (
	"database/sql"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/invoice"
	"github.com/nitrous-io/dedicated/nitrous/models/invoiceitem"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	stripe "github.com/stripe/stripe-go"
	stripeinvoice "github.com/stripe/stripe-go/invoice"
)

var (
	HandlePaymentSucceededFn func(*middleware.Env, *invoice.Invoice) error = handlePaymentSucceeded
)

func handleStripeInvoiceEvent(env *middleware.Env, e *stripe.Event, stripeID string) error {
	inv, err := invoice.FindByStripeID(env.DB, stripeID)
	if err != nil {
		invoiceInfo := ""
		if err == sql.ErrNoRows {
			strInv, err := stripeinvoice.Get(stripeID, nil)
			if err == nil {
				invoiceInfo = fmt.Sprintf("stripe invoice: %+v", strInv)
			}
			if containsProration(strInv) {
				logrus.Infof("handleStripeInvoiceEvent handling Stripe invoice with prorate items, will not commit to DB. Stripe invoice: %#v", strInv)
				return nil
			}
		}
		return fmt.Errorf("Error when locating invoice %s, err: %v, event: %+v, %s", stripeID, err, e, invoiceInfo)
	}

	// webhook data is found to be outdated, query the invoice for *live* info
	strInv, err := stripeinvoice.Get(stripeID, nil)
	if err != nil {
		return fmt.Errorf("Error requesting Stripe for invoice %s, err: %+v", stripeID, err)
	}

	if strInv.Paid {
		if err := HandlePaymentSucceededFn(env, inv); err != nil {
			return err
		}
	} else {
		i, err := subProvider.HandleSubscriptionInvoiceFailed(env.DB, stripeID)
		if err != nil {
			return fmt.Errorf("Error invoking HandleSubscriptionInvoiceFailed for Stripe invoice %s, err: %v", stripeID, err)
		} else if i != nil {
			inv = i
		}
	}

	u, err := user.FindByID(env.DB, inv.UserID)
	if err != nil {
		return fmt.Errorf("Error looking up user %d for invoice %d, err: %+v", inv.UserID, inv.ID, err)
	}

	go func() {
		if !strInv.Paid {
			eventName := fmt.Sprintf("Customer Card Declined %d", inv.Attempts)
			env.Tracker.TrackEvent(eventName, u, nil)
		}

		sInv, err := serializeInvoice(env.DB, inv)
		if err != nil {
			logrus.Errorf("Error serializing invoice %d for user %d, err: %+v", inv.UserID, inv.ID, err)
			return
		}
		env.Events.Publish(
			controllers.NewEvent(controllers.EventInvoiceUpdated, map[string]interface{}{
				"invoice": sInv,
			}),
			u.Topic(),
		)
	}()
	return nil
}

func serializeInvoice(d db.QueryReceiver, inv *invoice.Invoice) (*serializers.Invoice, error) {
	sInv := serializers.SerializeInvoice(inv)

	items := []invoiceitem.InvoiceItem{}
	err := d.Select(&items, "SELECT * FROM invoice_items WHERE invoice_id = $1 ORDER BY id DESC;", inv.ID)
	if err != nil {
		return nil, err
	}

	type hst struct {
		HostSlug  string `db:"host_slug"`
		PlanPrice int64  `db:"plan_price"`
	}
	hostWithPlan := &hst{}
	err = d.Get(hostWithPlan, `SELECT hosts.slug host_slug, plans.price plan_price
		FROM hosts INNER JOIN invoices ON invoices.subscription_id = hosts.subscription_id
		INNER JOIN plans ON hosts.plan_id = plans.id
		WHERE invoices.id = $1 ORDER BY hosts.id DESC;`, inv.ID)

	if err != nil {
		return nil, err
	}

	for _, item := range items {
		if item.Credit.Valid {
			sInv.TotalCredit += item.Credit.Int64
		} else {
			ser := serializers.SerializeInvoiceItem(&item)
			ser.HostSlug = hostWithPlan.HostSlug
			ser.Description = invoiceitem.ItemDescription(&item, &host.Host{Slug: hostWithPlan.HostSlug})
			ser.TotalCharge = item.TotalCharge(&plan.Plan{Price: hostWithPlan.PlanPrice})

			sInv.Items = append(sInv.Items, *ser)
		}
	}
	return sInv, nil
}

func handlePaymentSucceeded(env *middleware.Env, inv *invoice.Invoice) error {
	return inv.PaymentSucceeded(env.DB)
}

func containsProration(sInv *stripe.Invoice) bool {
	if sInv.Lines != nil {
		for _, invItem := range sInv.Lines.Values {
			if invItem.Proration {
				return true
			}
		}
	}

	return false
}
