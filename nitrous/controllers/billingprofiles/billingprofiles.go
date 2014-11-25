package billingprofiles

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Activate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cardToken := r.FormValue("card_token")
	if cardToken == "" {
		return httperror.New(http.StatusBadRequest, "card_token_required", "credit card token can't be blank")
	}

	bp, err := env.User.ActivateBillingProfile(env.DB, cardToken)
	if err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"billing_profile": serializers.SerializeBillingProfile(bp),
	}
	return nil
}
