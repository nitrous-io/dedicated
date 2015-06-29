package stripehooks

import (
	"database/sql"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
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

func handleInvoiceCreatedEvent(env *middleware.Env, e *stripe.Event, stripeID string) error {
	_, err := subProvider.HandleSubscriptionInvoiceCreated(env.DB, stripeID)
	return err
}

func publishSubscriptionEvent(env *middleware.Env, ev controllers.EventName, sub *subscription.Subscription) error {
	u := &user.User{}
	err := env.DB.Get(u, `SELECT users.* FROM users
		INNER JOIN billing_profiles ON users.id = billing_profiles.user_id
		INNER JOIN subscriptions ON subscriptions.billing_profile_id = billing_profiles.id
		WHERE subscriptions.id = $1`, sub.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	sWSlug := &subscription.SubscriptionWithHostSlug{
		Subscription: sub,
	}

	err = env.DB.Get(&sWSlug.HostSlug, `SELECT hosts.slug FROM hosts WHERE subscription_id = $1`, sub.ID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	env.Events.Publish(
		controllers.NewEvent(ev, map[string]interface{}{
			"subscription": serializers.SerializeSubscription(sWSlug),
		}),
		u.Topic(),
	)
	return nil
}
