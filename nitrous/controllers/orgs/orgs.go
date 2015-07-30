package orgs

import (
	"net/http"
	"strings"

	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errBannedEmail = httperror.New(http.StatusBadRequest, "email_banned", "this email address is banned")
)

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	user := env.User

	slug := strings.ToLower(r.FormValue("slug"))
	name := r.FormValue("name")
	email := strings.ToLower(r.FormValue("email"))

	o := &org.Org{
		Slug:  slug,
		Name:  name,
		Email: email,
	}

	err := o.Validate()
	if err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	// check email is not banned
	var banned bool
	err = db.Get(&banned, `SELECT COUNT(*) > 0 FROM users WHERE banned_at IS NOT NULL AND email = $1 LIMIT 1;`, o.Email)
	if err != nil {
		return err
	}
	if banned {
		return errBannedEmail
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	if err = o.Insert(tx); err != nil {
		tx.Rollback()
		return err
	}

	if err = o.AddMember(tx, user.ID, "owner"); err != nil {
		tx.Rollback()
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"org": serializers.SerializeOrg(o),
	}

	return nil
}
