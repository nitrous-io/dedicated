package appsettings

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"mixpanel_token": envvars.MixpanelToken,
		"stripe_key":     envvars.StripePublicKey,
	}

	// optionally include additional admin info but ignore auth failures
	tok, err := middleware.GetBearerToken(r)
	if err != nil {
		return nil
	}
	t, err := token.FindByToken(env.DB, tok)
	if err != nil {
		return nil
	}
	u, err := user.FindByID(env.DB, t.UserID)
	if err != nil {
		return nil
	}
	if u != nil && u.Admin {
		env.Data["freighterd_version"] = envvars.FreighterdVersion
	}
	return nil
}
