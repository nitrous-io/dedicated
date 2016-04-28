package billingprofiles

import (
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/controllers/shared"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/pkg/geoip"
	stripetoken "github.com/stripe/stripe-go/token"
)

var (
	IPLookupDB geoip.Locatable

	errCardCountryDifferent = httperror.New(http.StatusBadRequest, "bad_request", "Failed to authorize credit card")
)

func init() {
	// Initialize the IP lookup database asynchronously since it's not crucial -
	// if it's not yet downloaded, we will simply skip IP address verification.
	go initIPLookupDB()
}

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	bp, err := shared.BillingProfileForRequest(env, r)
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

	if err := verifyCard(cardToken, env.RemoteAddr); err != nil {
		return err
	}

	bp, err := shared.BillingProfileForRequest(env, r)
	if err != nil {
		return err
	}

	previousState := bp.State

	if bp.StripeID.Valid {
		err = bp.UpdateCard(db, cardToken)
		if err != nil {
			return shared.HandleBillingError(err)
		}
	} else {
		if bp.OrgID.Valid {
			var o *org.Org
			o, err = org.Find(db, bp.OrgID.Int64)
			if err != nil {
				return err
			}
			err = o.ActivateBillingProfile(db, bp, cardToken)
		} else {
			bp, err = u.ActivateBillingProfile(db, cardToken)
		}
		if err != nil {
			return shared.HandleBillingError(err)
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

	if err := verifyCard(cardToken, env.RemoteAddr); err != nil {
		return err
	}

	bp, err := shared.BillingProfileForRequest(env, r)
	if err != nil {
		return err
	}

	if err := bp.UpdateCard(db, cardToken); err != nil {
		return shared.HandleBillingError(err)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"billing_profile": serializers.SerializeBillingProfile(bp),
	}
	return nil
}

// Ensure that user IP is from the same country as that of the credit card to
// prevent fraud.
func verifyCard(cardToken, ip string) error {
	if IPLookupDB == nil {
		logrus.Warnf("failed to verify card - IP lookup database not initialized")
		return nil
	}

	// Retrieve card details from Stripe.
	t, err := stripetoken.Get(cardToken, nil)
	if err != nil {
		return err
	}

	if t.Card == nil || t.Card.CardCountry == "" {
		return nil
	}

	// Lookup user country from IP address. If lookup fails, we just log and
	// continue as if verification succeeded, since users could be
	// using anonymizer software.
	countryCode, err := IPLookupDB.LookupCountry(ip)
	if err != nil {
		logrus.Warnf("failed to lookup country of IP %q, err: %v", ip, err)
		return nil
	}

	if countryCode != t.Card.CardCountry {
		return errCardCountryDifferent
	}

	return nil
}

func initIPLookupDB() {
	db, err := geoip.New()
	if err != nil {
		logrus.Warnf("failed to initialize geoip database, err: %v", err)
		return
	}

	IPLookupDB = db
}
