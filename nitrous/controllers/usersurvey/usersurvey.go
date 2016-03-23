package usersurvey

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/bonus"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errBonusAlreadyEarned     = httperror.New(http.StatusForbidden, "forbidden", "you have already earned this bonus")
	errUserIneligibleForBonus = httperror.New(http.StatusForbidden, "ineligible_for_bonus", "you are not eligible for bonus")
	errInvalidHandshakeKey    = httperror.New(http.StatusForbidden, "forbidden", "invalid handshake key")
	errUsernameNotFound       = httperror.New(http.StatusNotFound, "username_not_found", "username not found")
)

// /api/v0/usersurvey/webhook_cb is the webhook URL that we store in the Wufoo form
// (Notification Settings). On submitting the form, a POST request is sent to this endpoint
// http://help.wufoo.com/articles/en_US/SurveyMonkeyArticleType/Webhooks
func WebhookCallback(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	handshakeKey := r.FormValue("HandshakeKey")
	if handshakeKey != envvars.WufooHandshakeKey {
		return errInvalidHandshakeKey
	}

	// Field113 is the field in the survey form that we prefill for user, which contains
	// the user's nitrous username that enables us to track the user in this callback
	// http://help.wufoo.com/articles/en_US/SurveyMonkeyArticleType/URL-Modifications
	username := r.FormValue("Field113")
	u, err := user.FindByUsername(env.DB, username)
	if err != nil {
		return errUsernameNotFound
	}

	bn, err := bonus.FindBySlug(env.DB, "complete-survey")
	if err != nil {
		if err == sql.ErrNoRows {
			bn, err = bonus.Insert(env.DB, "complete-survey", 18000)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if err := u.GrantBonus(env.DB, bn); err != nil {
		if err == bonus.ErrBonusAlreadyEarned {
			return errBonusAlreadyEarned
		}
		return errUserIneligibleForBonus
	}

	redirectUrl := fmt.Sprintf("https://%s/app/#/dashboard/earn-bonuses?completed_survey=true", envvars.WebHostname)

	http.Redirect(w, r, redirectUrl, http.StatusFound)

	return nil
}
