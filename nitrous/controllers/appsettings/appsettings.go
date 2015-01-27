package appsettings

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	var newUserCredit int64
	err := env.DB.QueryRow("SELECT default_value FROM credit_types WHERE name = 'Signup_Credit'").Scan(&newUserCredit)
	if err != nil {
		env.Logger.Errorf("appsettings: Error fetching default_value for Signup_Credit. err = %v", err)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"mixpanel_token":  envvars.MixpanelToken,
		"stripe_key":      envvars.StripePublicKey,
		"new_user_credit": newUserCredit,
	}
	return nil
}
