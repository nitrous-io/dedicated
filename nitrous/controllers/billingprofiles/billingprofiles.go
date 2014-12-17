package billingprofiles

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errStripeProfileRequired = httperror.New(http.StatusBadRequest, "billing_profile_not_active", "There's no existing card associated to this account.")

	errCardDeclined = httperror.New(http.StatusBadRequest, "card_declined", "Your Credit Card was declined. Try contacting the bank directly for assistance.")
	errCardExpired  = httperror.New(http.StatusBadRequest, "card_expired", "Your Credit Card has expired. Please try again with a recent card.")
	errIncorrectCvc = httperror.New(http.StatusBadRequest, "incorrect_cvc", "Your CVC is incorrect. Please try again.")
	errIncorrectZip = httperror.New(http.StatusBadRequest, "incorrect_zip", "Your Zip code is incorrect. Please try again.")
)

func Activate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cardToken := r.FormValue("card_token")
	if cardToken == "" {
		return httperror.New(http.StatusBadRequest, "card_token_required", "credit card token can't be blank")
	}

	bp, err := env.User.ActivateBillingProfile(env.DB, cardToken)
	if err != nil {
		return handleBillingError(err)
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
		return handleBillingError(err)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"billing_profile": serializers.SerializeBillingProfile(bp),
	}
	return nil
}

func handleBillingError(err error) error {
	switch err {
	case billingprofile.StripeProfileRequiredError:
		return errStripeProfileRequired
	case billingprofile.CardDeclinedError:
		return errCardDeclined
	case billingprofile.CardExpiredError:
		return errCardExpired
	case billingprofile.IncorrectCvcError:
		return errIncorrectCvc
	case billingprofile.IncorrectZipError:
		return errIncorrectZip
	default:
		return err
	}
}
