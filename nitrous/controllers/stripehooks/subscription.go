package stripehooks

import (
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
	err := subProvider.HandleSubscriptionUpdated(env.DB, env.Logger, stripeID)
	if err != nil {
		env.Logger.Errorf("Error calling HandleSubscriptionUpdated stripeID: %s, err: %v", stripeID, err)
	}
}

func handleSubscriptionDeletedEvent(env *middleware.Env, e *stripe.Event, stripeID string) {
	err := subProvider.HandleSubscriptionDeleted(env.DB, env.Logger, stripeID)
	if err != nil {
		env.Logger.Errorf("Error calling HandleSubscriptionDeleted stripeID: %s, err: %v", stripeID, err)
	}
}

func handleInvoiceCreatedEvent(env *middleware.Env, e *stripe.Event, stripeID string) {
	err := subProvider.HandleSubscriptionInvoiceCreated(env.DB, env.Logger, stripeID)
	if err != nil {
		env.Logger.Errorf("Error calling HandleSubscriptionInvoiceCreated stripeID: %s, err: %v", stripeID, err)
	}
}
