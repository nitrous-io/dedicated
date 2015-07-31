package billingprofiles

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/credittype"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
)

var (
	errStripeProfileRequired = httperror.New(http.StatusBadRequest, "billing_profile_not_active", "There's no existing card associated to this account.")

	errCardDeclined = httperror.New(http.StatusBadRequest, "card_declined", "Your Credit Card was declined. Try contacting the bank directly for assistance.")
	errCardExpired  = httperror.New(http.StatusBadRequest, "card_expired", "Your Credit Card has expired. Please try again with a recent card.")
	errIncorrectCvc = httperror.New(http.StatusBadRequest, "incorrect_cvc", "Your CVC is incorrect. Please try again.")
	errIncorrectZip = httperror.New(http.StatusBadRequest, "incorrect_zip", "Your Zip code is incorrect. Please try again.")
	errCardBanned   = httperror.New(http.StatusBadRequest, "card_banned", "Your credit card is banned.")

	errIncorrectSubscriptionRole = httperror.New(http.StatusForbidden, "incorrect_role", "You don't have permission to access this subscription")
)

func billingProfileForRequest(env *middleware.Env, r *http.Request) (*billingprofile.BillingProfile, error) {
	if orgSlug := mux.Vars(r)["org_slug"]; orgSlug != "" {
		mem, err := orgmembership.FindByUsernameAndOrgSlug(env.DB, env.User.Username, orgSlug)
		if err != nil {
			return nil, err
		}
		if mem.Role != "owner" {
			return nil, errIncorrectSubscriptionRole
		}
		return billingprofile.FindForOrg(env.DB, mem.OrgID)
	}

	return billingprofile.Find(env.DB, env.User.ID)
}

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	bp, err := billingProfileForRequest(env, r)
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

	bp, err := billingProfileForRequest(env, r)
	if err != nil {
		return err
	}

	previousState := bp.State
	previousPendingActivation := bp.PendingActivation

	if bp.StripeID.Valid {
		err = bp.UpdateCard(db, cardToken)
		if err != nil {
			return handleBillingError(err)
		}
	} else {
		if bp.OrgID.Valid {
			var o *org.Org
			o, err = org.Find(db, bp.OrgID.Int64)
			if err != nil {
				return err
			}
			err = activateOrgBillingProfile(db, o, bp, cardToken)
		} else {
			bp, err = u.ActivateBillingProfile(db, cardToken)
		}
		if err != nil {
			return handleBillingError(err)
		}
	}

	// we have had a lot of stolen credit cards used from specific countries.
	// Use a CIDR blacklist to set user to delinquent and inform us (so we
	// still can review their info on Stripe and manually update their billing
	// profile if they are legit)
	ip := middleware.GetRemoteAddr(r)
	if err := billingprofile.CIDRBlacklisted(ip); err != nil {
		bp.State = billingprofile.StateDelinquent
		logrus.Warnf("CIDRBlacklisted error on user %s ip %s: %v", u.Username, ip, err)
		if err := bp.Update(db); err != nil {
			// since this part is technically optional, we log any errors
			// encountered and continue
			logrus.Errorf("Error updating CIDRBlacklisted billing profile to delinquent: %v", err)
		}
	}

	ser := serializers.SerializeBillingProfile(bp)
	defer func() {
		env.Status = http.StatusCreated
		env.Data = map[string]interface{}{
			"billing_profile": ser,
		}
	}()

	// all other operates are user focused, we skip these if for a Organisation
	if bp.OrgID.Valid {
		return nil
	}

	// determine if this user gets a credit
	if previousPendingActivation && !bp.PendingActivation && bp.StripeFingerprint.String != "" {
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM billing_profiles WHERE stripe_fingerprint = $1;", bp.StripeFingerprint).Scan(&count)
		if err != nil {
			logrus.Errorf("Error locating similar Stripe fingerprints, err: %v", err)
		} else if count <= 1 {
			ser.AmountCredited = awardCredits(db, u)
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

	return nil
}

func Update(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	cardToken := r.FormValue("card_token")
	if cardToken == "" {
		return httperror.New(http.StatusBadRequest, "card_token_required", "credit card token can't be blank")
	}

	bp, err := billingProfileForRequest(env, r)
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

func activateOrgBillingProfile(d *db.Database, o *org.Org, bp *billingprofile.BillingProfile, token string) error {
	params := &stripe.CustomerParams{
		Email: o.Email,
		Desc:  fmt.Sprintf("Organisation %q", o.Name),
		Params: stripe.Params{
			Meta: map[string]string{
				"name": o.Name,
			},
		},
	}
	if token != "" {
		params.Token = token
	}
	c, err := customer.New(params)
	if err != nil {
		return billingprofile.HandleStripeError(err)
	}

	return bp.Activate(d, c)
}
