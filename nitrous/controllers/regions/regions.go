package regions

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/appconfig"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	rs, err := region.AllForProvider(env.DB, appconfig.DefaultProvider)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"regions": serializers.SerializeRegions(rs),
	}
	return nil
}
