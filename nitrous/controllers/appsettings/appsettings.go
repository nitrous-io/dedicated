package appsettings

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/appconfig"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/token"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"mixpanel_token": appconfig.MixpanelToken,
		"stripe_key":     appconfig.StripePublicKey,
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
		env.Data["freighterd_version"] = appconfig.FreighterdVersion
	}
	return nil
}
