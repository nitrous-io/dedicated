package userprofiles

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
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
		bp, err := billingprofile.Find(db, env.User.ID)
		if err != nil {
			return err
		}
		// free users and those without credit card are not allowed to change Advanced state
		if bp.IsFreeOrCardRequired() {
			return errors.New("User not allowed to change Advanced state")
		}
		p.Advanced = v

		err = p.Update(db)
		if err != nil {
			return err
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"user_profile": serializers.SerializeUserProfile(p),
	}
	return nil
}
