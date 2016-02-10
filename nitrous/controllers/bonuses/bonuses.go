package bonuses

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	bns, err := bonus.FindAll(env.DB)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"bonuses": serializers.SerializeBonuses(bns),
	}

	return nil
}

func EarnedByUser(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	ebns, err := bonus.FindEarnedByUser(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"earned_bonuses": serializers.SerializeEarnedBonuses(ebns),
	}

	return nil
}
