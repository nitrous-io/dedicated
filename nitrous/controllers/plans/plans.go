package plans

import (
	"net/http"
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var provider = envvars.DefaultProvider

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

func showPlans(d *db.Database, u *user.User, showAll bool) ([]*plan.Plan, error) {
	if u == nil || showAll {
		return plan.AllPlansForProvider(d, provider)
	} else {
		return freetier.EligiblePlansForUser(d, u, provider)
	}
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	u := optionalLogin(env, w, r)

	showAll, _ := strconv.ParseBool(r.FormValue("all"))

	pls, err := showPlans(env.DB, u, showAll)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"plans": serializers.SerializePlans(pls),
	}
	return nil
}
