package users

import "github.com/nitrous-io/dedicated/nitrous/subscriptions"

func SetSubscriptionProvider(s subscriptions.Provider) {
	subProvider = s
}
