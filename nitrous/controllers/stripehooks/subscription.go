package stripehooks

import (
	"database/sql"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
	stripe "github.com/stripe/stripe-go"
)

var (
	subProvider subscriptions.Provider
)

func init() {
	subProvider = subscriptions.NewProvider()
}

func handleSubscriptionUpdatedEvent(env *middleware.Env, e *stripe.Event, stripeID string) error {
	sub, err := subProvider.HandleSubscriptionUpdated(env.DB, stripeID)
	if err != nil {
		return err
	}

	go func() {
		err := publishSubscriptionEvent(env, controllers.EventSubscriptionUpdated, sub)
		if err != nil {
			logrus.Errorf("Error publishing EventSubscriptionUpdated for sub %d, err: %v", sub.ID, err)
		}
	}()

	return nil
}

func handleSubscriptionDeletedEvent(env *middleware.Env, e *stripe.Event, stripeID string) error {
	sub, err := subProvider.HandleSubscriptionDeleted(env.DB, stripeID)
	if err != nil {
		return err
	}

	go func() {
		err := publishSubscriptionEvent(env, controllers.EventSubscriptionDeleted, sub)
		if err != nil {
			logrus.Errorf("Error publishing EventSubscriptionDeleted for sub %d, err: %v", sub.ID, err)
		}
	}()

	return nil
}

func handleChargeSucceededEvent(env *middleware.Env, e *stripe.Event, stripeID string) error {
	return subProvider.HandleChargeSucceeded(env.DB, e)
}

func handleChargeFailedEvent(env *middleware.Env, e *stripe.Event, stripeID string) error {
	return subProvider.HandleChargeFailed(env.DB, e)
}

func handleInvoiceCreatedEvent(env *middleware.Env, e *stripe.Event, stripeID string) error {
	_, err := subProvider.HandleSubscriptionInvoiceCreated(env.DB, stripeID)
	return err
}

func publishSubscriptionEvent(env *middleware.Env, ev controllers.EventName, sub *subscription.Subscription) error {
	bp, err := billingprofile.FindByID(env.DB, sub.BillingProfileID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	sWSlug := &subscription.SubscriptionWithHostSlug{
		Subscription: sub,
		OrgID: bp.OrgID,
	}

	err = env.DB.Get(&sWSlug.HostSlug, `SELECT hosts.slug FROM hosts WHERE subscription_id = $1`, sub.ID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	subSer := serializers.SerializeSubscription(sWSlug)

	pl, err := plan.FindByID(env.DB, sub.PlanID)
	if err != nil {
		logrus.Errorf("Error locating plan for Subscription %d, err: %v", sub.ID, err)
	} else {
		subSer.Plan = serializers.SerializePlan(pl)
	}

	env.Events.Publish(
		controllers.NewEvent(ev, map[string]interface{}{
			"subscription": subSer,
		}),
		pubsub.Topic(fmt.Sprintf("user-%d", bp.UserID)),
	)
	return nil
}
