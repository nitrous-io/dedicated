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

var (
	FreeCreditName = "Signup_Credit"
)

func Activate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	cardToken := r.FormValue("card_token")
	if cardToken == "" {
		return httperror.New(http.StatusBadRequest, "card_token_required", "credit card token can't be blank")
	}

	existing, err := billingprofile.Find(db, env.User.ID)
	if err != nil {
		return err
	}

	bp, err := env.User.ActivateBillingProfile(db, cardToken)
	if err != nil {
		return handleBillingError(err)
	}

	// determine if this user gets a credit
	if existing.PendingActivation && !bp.PendingActivation && bp.StripeFingerprint.String != "" {
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM billing_profiles WHERE stripe_fingerprint = $1;", bp.StripeFingerprint).Scan(&count)
		if err != nil {
			env.Logger.Errorf("Error locating similar Stripe fingerprints, err: %+v", err)
		} else if count <= 1 {
			_, err = db.Exec(`INSERT INTO credits(user_id, credit_type_id, amount) SELECT $1, id, default_value FROM credit_types WHERE active = true AND name = $2;`, env.User.ID, FreeCreditName)
			if err != nil {
				env.Logger.Errorf("Error awarding credits, err: %+v", err)
			}
		}
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
