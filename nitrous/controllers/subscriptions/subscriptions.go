package subscriptions

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	sub "github.com/nitrous-io/dedicated/nitrous/subscriptions"
)

var (
	subProvider sub.Provider
)

func init() {
	subProvider = sub.NewProvider()
}

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

func Cancel(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		return err
	}

	sub := &subscription.Subscription{}
	err = env.DB.Get(sub, "SELECT * FROM subscriptions WHERE id = $1;", id)
	if err != nil {
		return err
	}

	err = subProvider.Cancel(env.DB, sub)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "cancelled",
	}
	return nil
}
