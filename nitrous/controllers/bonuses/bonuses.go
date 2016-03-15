package bonuses

import (
	"database/sql"
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errInvalidBonusAction     = httperror.New(http.StatusNotFound, "bonus_not_found", "the bonus type is invalid")
	errBonusAlreadyEarned     = httperror.New(http.StatusForbidden, "forbidden", "you have already earned this bonus")
	errUserIneligibleForBonus = httperror.New(http.StatusForbidden, "user_ineligible_for_bonus", "you are not eligible for bonus")
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

// Not abuse-proof (e.g. users can just send POST requests with the correct action to earn bonus hours)
// TODO: We should never need this endpoint.
func Earn(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	u := env.User
	action := r.PostFormValue("action")

	bn, err := bonus.FindBySlug(env.DB, action)
	if err != nil {
		if err == sql.ErrNoRows {
			return errInvalidBonusAction
		}
		return err
	}

	if err := u.GrantBonus(env.DB, bn); err != nil {
		if err == bonus.ErrBonusAlreadyEarned {
			return errBonusAlreadyEarned
		}
		return errUserIneligibleForBonus
	}

	ebns, err := bonus.FindEarnedByUser(env.DB, u.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"earned_bonuses": serializers.SerializeEarnedBonuses(ebns),
	}

	return nil
}
