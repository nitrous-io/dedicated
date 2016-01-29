package plans

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	u, db := env.User, env.DB

	bp, err := u.BillingProfile(db)
	if err != nil {
		return err
	}

	finder := plan.PaidPlansForProvider
	switch {
	case bp.IsNoCard():
		// Show all plans if user has not yet added a credit card.
		finder = plan.AllPlansForProvider
	case bp.IsFreeTier():
		// Show all plans if user is in free tier and has no existing hosts.
		hosts, err := host.FindAllWithoutOrphaned(db, u.ID)
		if err != nil {
			return err
		}
		if len(hosts) == 0 {
			finder = plan.AllPlansForProvider
		}
	}

	plans, err := finder(db, envvars.DefaultProvider)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"plans": serializers.SerializePlans(plans),
	}
	return nil
}

func All(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	plans, err := plan.AllPlansForProvider(env.DB, envvars.DefaultProvider)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"plans": serializers.SerializePlans(plans),
	}
	return nil
}
