package plans

import (
	"net/http"

	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/plan"
	"github.com/nitrous-io/nitrous/serializers"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	pls, err := plan.All(env.DB)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
		"plans":  serializers.SerializePlans(pls),
	}
	return nil
}
