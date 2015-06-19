package subscriptions

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	subscriptions, err := subscription.FindAllActiveByUserID(env.DB, env.User.ID)
	if err != nil {
		return err
	}
	serializedSubscriptions := []*serializers.Subscription{}

	for _, s := range subscriptions {
		serializedSubscription := serializers.SerializeSubscription(s)
		serializedSubscriptions = append(serializedSubscriptions, serializedSubscription)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"subscriptions": serializedSubscriptions,
	}
	return nil
}
