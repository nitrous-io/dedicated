package plans

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	pls, err := freetier.EligiblePlansForUser(env.DB, env.User, envvars.DefaultProvider)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"plans": serializers.SerializePlans(pls),
	}
	return nil
}
