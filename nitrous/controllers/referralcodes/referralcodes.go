package referralcodes

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/referralcode"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	rc, err := referralcode.FindByUserID(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"referral_code": serializers.SerializeReferralCode(rc),
	}

	return nil
}
