package plans

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	plans, err := freetier.EligiblePlansForUser(env.DB, env.User, envvars.DefaultProvider)
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
