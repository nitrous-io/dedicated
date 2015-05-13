package billingprofiles

import (
	"database/sql"
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/freequeue"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/nitrous-common/logger"
)

var (
	errStripeProfileRequired = httperror.New(http.StatusBadRequest, "billing_profile_not_active", "There's no existing card associated to this account.")

	errCardDeclined = httperror.New(http.StatusBadRequest, "card_declined", "Your Credit Card was declined. Try contacting the bank directly for assistance.")
	errCardExpired  = httperror.New(http.StatusBadRequest, "card_expired", "Your Credit Card has expired. Please try again with a recent card.")
	errIncorrectCvc = httperror.New(http.StatusBadRequest, "incorrect_cvc", "Your CVC is incorrect. Please try again.")
	errIncorrectZip = httperror.New(http.StatusBadRequest, "incorrect_zip", "Your Zip code is incorrect. Please try again.")
)

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	bp, err := billingprofile.Find(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"billing_profile": serializers.SerializeBillingProfile(bp),
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

	existing, err := billingprofile.Find(db, u.ID)
	if err != nil {
		return err
	}

	bp, err := u.ActivateBillingProfile(db, cardToken)
	if err != nil {
		return handleBillingError(err)
	}

	// determine if this user gets a credit
	if existing.PendingActivation && !bp.PendingActivation && bp.StripeFingerprint.String != "" {
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM billing_profiles WHERE stripe_fingerprint = $1;", bp.StripeFingerprint).Scan(&count)
		if err != nil {
			env.Logger.Errorf("Error locating similar Stripe fingerprints, err: %v", err)
		} else if count <= 1 {
			awardCredits(db, env.Logger, u)
		}
	}

	// Remove any freequeue entries (since user has already paid).
	if err := freequeue.Cancel(db, u.ID); err != nil {
		env.Logger.Errorf("Error canceling freequeue entries: %v", err)
	}

	if err := freetier.UpgradeFreeUserHosts(db, u); err != nil {
		env.Logger.Errorf("Error upgrading free-tier hosts of paid user: %v", err)
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

func awardCredits(d *db.Database, l *logger.Logger, user *user.User) {
	if user.PromoCode.String == "" || !user.PromoCode.Valid {
		// promo_code is '' or NULL, return
		return
	}

	ct, err := creditForUser(d, user)
	if err != nil {
		if err == sql.ErrNoRows {
			l.Errorf("Credit '%s' does not exist", user.PromoCode.String)
			return
		}
		l.Errorf("Error locating credit for user, err: %+v", err)
		return
	}

	tx, err := d.Beginx()
	if err != nil {
		l.Errorf("Error opening db transaction, err: %+v", err)
		return
	}

	_, err = tx.Exec(`INSERT INTO credits(user_id, credit_type_id, amount) VALUES ($1, $2, $3);`, user.ID, ct.ID, ct.DefaultValue)
	if err != nil {
		tx.Rollback()
		l.Errorf("Error locating credit for user, err: %+v", err)
		return
	}

	_, err = tx.Exec(`UPDATE credit_types SET used = coalesce(used, 0) + 1 WHERE id = $1`, ct.ID)
	if err != nil {
		tx.Rollback()
		l.Errorf("Error incrementing used credit, err: %+v", err)
		return
	}

	err = tx.Commit()
	if err != nil {
		l.Errorf("Error committing credit for user, err: %+v", err)
	}
}

func creditForUser(d db.QueryReceiver, user *user.User) (*credittype.CreditType, error) {
	creditCode := user.PromoCode.String
	return credittype.FindValidCreditTypeByName(d, creditCode)
}
