package appsettings

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"mixpanel_token": envvars.MixpanelToken,
		"stripe_key":     envvars.StripePublicKey,
	}

	return nil
}
