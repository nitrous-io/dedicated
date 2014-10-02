package regions

import (
	"net/http"

	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/region"
	"github.com/nitrous-io/nitrous/serializers"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	rs, err := region.All(env.DB)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"regions": serializers.SerializeRegions(rs),
	}
	return nil
}
