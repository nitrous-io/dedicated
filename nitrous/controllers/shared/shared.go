package shared

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errIncorrectSubscriptionRole = httperror.New(http.StatusForbidden, "incorrect_role", "You don't have permission to access this subscription")
)

func BillingProfileForRequest(env *middleware.Env, r *http.Request) (*billingprofile.BillingProfile, error) {
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
