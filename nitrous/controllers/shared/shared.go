package shared

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
)

var (
	errIncorrectSubscriptionRole = httperror.New(http.StatusForbidden, "incorrect_role", "You don't have permission to access this subscription")

	errStripeProfileRequired = httperror.New(http.StatusBadRequest, "billing_profile_not_active", "There's no existing card associated to this account.")

	errCardDeclined = httperror.New(http.StatusBadRequest, "card_declined", "Your credit card was declined. Try contacting the bank directly for assistance.")
	errCardExpired  = httperror.New(http.StatusBadRequest, "card_expired", "Your credit card has expired. Please try again with a recent card.")
	errIncorrectCvc = httperror.New(http.StatusBadRequest, "incorrect_cvc", "Your CVC is incorrect. Please try again.")
	errIncorrectZip = httperror.New(http.StatusBadRequest, "incorrect_zip", "Your Zip code is incorrect. Please try again.")
	errCardBanned   = httperror.New(http.StatusBadRequest, "card_banned", "Your credit card is banned.")

	errExistingMultiHostSubscription = httperror.New(http.StatusBadRequest, "upgrade_plan", "You already have a subscription, please upgrade.")
)

func BillingProfileForRequest(env *middleware.Env, r *http.Request) (*billingprofile.BillingProfile, error) {
	if orgSlug := mux.Vars(r)["org_slug"]; orgSlug != "" {
		mem, err := orgmembership.FindByIdentifierAndOrgSlug(env.DB, env.User.Username, orgSlug)
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

func HandleBillingError(err error) error {
	switch err {
	case billingprofile.StripeProfileRequiredError:
		return errStripeProfileRequired
	case billingprofile.ErrCardDeclined, subscriptions.ErrCardDeclined:
		return errCardDeclined
	case billingprofile.ErrCardExpired, subscriptions.ErrCardExpired:
		return errCardExpired
	case billingprofile.ErrIncorrectCvc:
		return errIncorrectCvc
	case billingprofile.ErrIncorrectZipErr:
		return errIncorrectZip
	case user.ErrCardBanned, billingprofile.ErrCardBanned:
		return errCardBanned
	case subscriptions.ErrExistingMultiHostSubscription:
		return errExistingMultiHostSubscription
	default:
		return err
	}
}
