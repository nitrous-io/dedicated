package userprofiles

import (
	"net/http"
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/userprofile"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	p, err := userprofile.Find(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user_profile": serializers.SerializeUserProfile(p),
	}
	return nil
}

func Update(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	p, err := userprofile.Find(db, env.User.ID)
	if err != nil {
		return err
	}

	if v, err := strconv.ParseBool(r.FormValue("advanced")); err == nil {
		p.Advanced = v

		if err := p.Update(db); err != nil {
			return err
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user_profile": serializers.SerializeUserProfile(p),
	}
	return nil
}
