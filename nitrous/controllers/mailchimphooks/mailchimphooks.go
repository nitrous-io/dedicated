package mailchimphooks

import (
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
)

func VerifyWebhook(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func ReceiveHook(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	processMailChimpHook(env.DB, r.FormValue("data[list_id]"), r.FormValue("data[email]"), r.FormValue("type"))
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

func processMailChimpHook(d *db.Database, listID, email, action string) {
	subscribed := action == "subscribe"
	var targetColumn string
	switch listID {
	case envvars.MailChimpIDNewsletter:
		targetColumn = "subscribe_newsletter"
	case envvars.MailChimpIDAnnouncements:
		targetColumn = "subscribe_announcements"
	default:
		logrus.Errorf("MailChimp sent an unrecognized listID: %s, email: %s, action: %s", listID, email, action)
		return
	}
	query := fmt.Sprintf(`UPDATE user_profiles
                  SET ("%s", updated_at) = ($1, now())
                  WHERE user_id = (SELECT id FROM users WHERE email = $2 AND deleted_at IS NULL);`, targetColumn)
	_, err := d.Exec(query, subscribed, email)
	if err != nil {
		logrus.Errorf("Error updating MailChimp list %s for email %s, err: %v", targetColumn, email, err)
	}
}
