package plans

import (
	"fmt"
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	pls, err := plan.EnabledAndForProvider(env.DB, envvars.DefaultProvider)
	if err != nil {
		fmt.Printf("plan.EnabledAndProvider('%s') failed with %s\n", envvars.DefaultProvider, err)
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"plans": serializers.SerializePlans(pls),
	}
	return nil
}
