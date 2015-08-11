package billingprofiles

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errStripeProfileRequired = httperror.New(http.StatusBadRequest, "billing_profile_not_active", "There's no existing card associated to this account.")

	errCardDeclined = httperror.New(http.StatusBadRequest, "card_declined", "Your Credit Card was declined. Try contacting the bank directly for assistance.")
	errCardExpired  = httperror.New(http.StatusBadRequest, "card_expired", "Your Credit Card has expired. Please try again with a recent card.")
	errIncorrectCvc = httperror.New(http.StatusBadRequest, "incorrect_cvc", "Your CVC is incorrect. Please try again.")
	errIncorrectZip = httperror.New(http.StatusBadRequest, "incorrect_zip", "Your Zip code is incorrect. Please try again.")
	errCardBanned   = httperror.New(http.StatusBadRequest, "card_banned", "Your credit card has been banned.")
)

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	bp, err := billingprofile.Find(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	ser := serializers.SerializeBillingProfile(bp)
	if bp.State == billingprofile.StateFree && env.User.PromoCode.Valid && env.User.PromoCode.String != "" {
		var amount int64
		err := env.DB.Get(&amount, "SELECT default_value FROM credit_types WHERE name = $1;", env.User.PromoCode.String)
		if err != nil {
			logrus.Errorf("Error finding promo code %s err: %+v", env.User.PromoCode.String, err)
		}
		ser.AmountCredited = amount
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"billing_profile": ser,
	}
	return nil
}

func Activate(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	cardToken := r.FormValue("card_token")
	if cardToken == "" {
		return httperror.New(http.StatusBadRequest, "card_token_required", "credit card token can't be blank")
	}

	bp, err := billingprofile.Find(db, u.ID)
	if err != nil {
		return err
	}

	previousState := bp.State
	previousPendingActivation := bp.PendingActivation

	if bp.StripeID.Valid {
		err = bp.UpdateCard(db, cardToken)
		if err != nil {
			fmt.Printf("Err: %+v\n", err)
			return handleBillingError(err)
		}
	} else {
		bp, err = u.ActivateBillingProfile(db, cardToken)
		if err != nil {
			return handleBillingError(err)
		}
	}

	if u.Suspicious.Valid {
		bp.State = billingprofile.StateDelinquent
		if err := bp.Update(db); err != nil {
			// since this part is technically optional, we log any errors
			// encountered and continue
			logrus.Errorf("Error updating suspicious user to delinquent: %v", err)
		}
	}

	// determine if this user gets a credit
	var credited int64
	if previousPendingActivation && !bp.PendingActivation && bp.StripeFingerprint.String != "" {
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM billing_profiles WHERE stripe_fingerprint = $1;", bp.StripeFingerprint).Scan(&count)
		if err != nil {
			logrus.Errorf("Error locating similar Stripe fingerprints, err: %v", err)
		} else if count <= 1 {
			credited = awardCredits(db, u)
		}
	}

	if (previousState == billingprofile.StateFree || previousState == billingprofile.StateNoCard) &&
		bp.State == billingprofile.StatePaid {
		env.Tracker.SetProfileProps("set", env.User.ID, map[string]interface{}{
			"upgraded_to_paid_from": previousState,
		})
		env.Tracker.SetProfileProps("set", env.User.ID, map[string]interface{}{
			"upgraded_to_paid_at": time.Now(),
		})
		env.AddEventData(map[string]interface{}{"upgraded_to_paid_from": previousState})
	}

	ser := serializers.SerializeBillingProfile(bp)
	ser.AmountCredited = credited

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"billing_profile": ser,
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
	case billingprofile.ErrCardDeclined:
		return errCardDeclined
	case billingprofile.ErrCardExpired:
		return errCardExpired
	case billingprofile.ErrIncorrectCvc:
		return errIncorrectCvc
	case billingprofile.ErrIncorrectZipErr:
		return errIncorrectZip
	case user.ErrCardBanned, billingprofile.ErrCardBanned:
		return errCardBanned
	default:
		return err
	}
}

func awardCredits(d *db.Database, user *user.User) int64 {
	if user.PromoCode.String == "" || !user.PromoCode.Valid {
		// promo_code is '' or NULL, return
		return 0
	}

	ct, err := creditForUser(d, user)
	if err != nil {
		if err == sql.ErrNoRows {
			logrus.Errorf("Credit '%s' does not exist", user.PromoCode.String)
			return 0
		}
		logrus.Errorf("Error locating credit for user, err: %+v", err)
		return 0
	}

	tx, err := d.Beginx()
	if err != nil {
		logrus.Errorf("Error opening db transaction, err: %+v", err)
		return 0
	}

	_, err = tx.Exec(`INSERT INTO credits(user_id, credit_type_id, amount) VALUES ($1, $2, $3);`, user.ID, ct.ID, ct.DefaultValue)
	if err != nil {
		tx.Rollback()
		logrus.Errorf("Error locating credit for user, err: %+v", err)
		return 0
	}

	_, err = tx.Exec(`UPDATE credit_types SET used = coalesce(used, 0) + 1 WHERE id = $1`, ct.ID)
	if err != nil {
		tx.Rollback()
		logrus.Errorf("Error incrementing used credit, err: %+v", err)
		return 0
	}

	err = tx.Commit()
	if err != nil {
		logrus.Errorf("Error committing credit for user, err: %+v", err)
		return 0
	}

	return ct.DefaultValue
}

func creditForUser(d db.QueryReceiver, user *user.User) (*credittype.CreditType, error) {
	creditCode := user.PromoCode.String
	return credittype.FindValidCreditTypeByName(d, creditCode)
}
