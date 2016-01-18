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
	var (
		plans []*plan.Plan
	)
	u, db := env.User, env.DB

	bp, err := u.BillingProfile(db)
	if err != nil {
		return err
	}

	switch {
	case bp.IsNoCard():
		// Show all plans if user has not yet added a credit card.
		plans, err = plan.AllPlansForProvider(db, envvars.DefaultProvider)
		if err != nil {
			return err
		}
	case bp.IsFreeTier():
		// If user is already in free tier and has an existing host,
		// we only allow them to create a paid host
		hosts, err := host.FindAllWithoutOrphaned(db, u.ID)
		if err != nil {
			return err
		}
		if len(hosts) > 0 {
			plans, err = plan.PaidPlansForProvider(db, envvars.DefaultProvider)
			if err != nil {
				return err
			}
		} else {
			plans, err = plan.AllPlansForProvider(db, envvars.DefaultProvider)
			if err != nil {
				return err
			}
		}
	default:
		plans, err = plan.PaidPlansForProvider(db, envvars.DefaultProvider)
		if err != nil {
			return err
		}
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
