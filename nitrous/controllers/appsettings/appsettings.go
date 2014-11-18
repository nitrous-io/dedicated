package appsettings

import (
	"net/http"
	"os"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"mixpanel_token": os.Getenv("MIXPANEL_TOKEN"),
	}
	return nil
}
