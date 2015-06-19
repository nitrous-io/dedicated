package stripehooks

import (
	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
	stripe "github.com/stripe/stripe-go"
)

var (
	subProvider subscriptions.Provider
)

func init() {
	subProvider = subscriptions.NewProvider()
}

func handleSubscriptionUpdatedEvent(env *middleware.Env, e *stripe.Event, stripeID string) {
	err := subProvider.HandleSubscriptionUpdated(env.DB, stripeID)
	if err != nil {
		logrus.Errorf("Error calling HandleSubscriptionUpdated stripeID: %s, err: %v", stripeID, err)
	}
}

func handleSubscriptionDeletedEvent(env *middleware.Env, e *stripe.Event, stripeID string) {
	err := subProvider.HandleSubscriptionDeleted(env.DB, stripeID)
	if err != nil {
		logrus.Errorf("Error calling HandleSubscriptionDeleted stripeID: %s, err: %v", stripeID, err)
	}
}

func handleInvoiceCreatedEvent(env *middleware.Env, e *stripe.Event, stripeID string) {
	err := subProvider.HandleSubscriptionInvoiceCreated(env.DB, stripeID)
	if err != nil {
		logrus.Errorf("Error calling HandleSubscriptionInvoiceCreated stripeID: %s, err: %v", stripeID, err)
	}
}
