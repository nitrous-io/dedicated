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

func handleSubscriptionUpdatedEvent(env *middleware.Env, e *stripe.Event, stripeID string) error {
	return subProvider.HandleSubscriptionUpdated(env.DB, stripeID)
}

func handleSubscriptionDeletedEvent(env *middleware.Env, e *stripe.Event, stripeID string) error {
	return subProvider.HandleSubscriptionDeleted(env.DB, stripeID)
}

func handleInvoiceCreatedEvent(env *middleware.Env, e *stripe.Event, stripeID string) error {
	return subProvider.HandleSubscriptionInvoiceCreated(env.DB, stripeID)
}
