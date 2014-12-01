package billingprofiles

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	ErrStripeProfileRequired = httperror.New(http.StatusBadRequest, "credit_card_required", "You need to add a credit card to create an application.")
	ErrCardDeclined          = httperror.New(http.StatusBadRequest, "card_declined", "Your Credit Card was declined. Try contacting the bank directly for assistance.")
)

func Activate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cardToken := r.FormValue("card_token")
	if cardToken == "" {
		return httperror.New(http.StatusBadRequest, "card_token_required", "credit card token can't be blank")
	}

	bp, err := env.User.ActivateBillingProfile(env.DB, cardToken)
	if err != nil {
		if err == billingprofile.CardDeclinedError {
			return ErrCardDeclined
		}
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"billing_profile": serializers.SerializeBillingProfile(bp),
	}
	return nil
}

func Update(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	cardToken := r.FormValue("card_token")
	if cardToken == "" {
		return httperror.New(http.StatusBadRequest, "card_token_required", "credit card token can't be blank")
	}

	bp, err := billingprofile.Find(db, env.User.ID)
	if err != nil {
		return err
	}

	if err := bp.UpdateCard(db, cardToken); err != nil {
		switch err {
		case billingprofile.StripeProfileRequiredError:
			return ErrStripeProfileRequired
		case billingprofile.CardDeclinedError:
			return ErrCardDeclined
		default:
			return err
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"billing_profile": serializers.SerializeBillingProfile(bp),
	}
	return nil
}
