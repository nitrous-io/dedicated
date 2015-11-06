package plans

import (
	"database/sql"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	provider = envvars.DefaultProvider

	errOrgNotFound = httperror.New(http.StatusNotFound, "org_not_found", "the org does not exist")
)

func optionalLogin(env *middleware.Env, w http.ResponseWriter, r *http.Request) (u *user.User) {
	token, err := middleware.GetBearerToken(r)
	if err != nil {
		return nil
	}

	err = middleware.Authenticate(token, env, w, r)
	if err != nil {
		return nil
	}

	return env.User
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	u := optionalLogin(env, w, r)

	orgSlug := mux.Vars(r)["org_slug"]
	var plans []*plan.Plan

	if orgSlug != "" {
		var err error
		plans, err = orgPlans(env, u, orgSlug)
		if err != nil {
			return err
		}
	} else {
		var err error
		plans, err = plan.PlansFor(env.DB, u)
		if err != nil {
			return err
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"plans": serializers.SerializePlans(plans),
	}
	return nil
}

func orgPlans(env *middleware.Env, u *user.User, orgSlug string) ([]*plan.Plan, error) {
	o, err := org.FindByIdOrSlug(env.DB, orgSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errOrgNotFound
		}
		return nil, err
	}

	_, err = orgmembership.FindByUsernameAndOrgSlug(env.DB, u.Username, o.Slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errOrgNotFound
		}
		return nil, err
	}

	return o.Plans(env.DB)
}
