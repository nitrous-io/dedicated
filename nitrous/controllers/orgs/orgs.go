package orgs

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errBannedEmail  = httperror.New(http.StatusBadRequest, "email_banned", "this email address is banned")
	errNotFound     = httperror.New(http.StatusNotFound, "not_found", "user or org not found")
	errNoPermission = httperror.New(http.StatusForbidden, "permission_denied", "you do not have permission to add or update member")
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

	if err = orgmembership.AddMember(tx, o.ID, user.ID, "owner"); err != nil {
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

func PutMember(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	currentUser := env.User
	slug := mux.Vars(r)["slug"]
	username := mux.Vars(r)["username"]

	if slug == "" || username == "" {
		return errNotFound
	}

	role := strings.ToLower(r.FormValue("role"))
	if _, ok := orgmembership.ValidRoles[role]; !ok {
		return httperror.New(http.StatusBadRequest, "invalid_role", fmt.Sprintf("%s is not a valid role", role))
	}

	o, err := org.FindBySlug(db, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	u, err := user.FindByUsername(db, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
	}

	cuMembership, err := orgmembership.FindByUsernameAndOrgSlug(db, currentUser.Username, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNoPermission
		}
	}

	permissableRoles, ok := orgmembership.ValidRoles[cuMembership.Role]
	if !ok {
		return httperror.New(http.StatusBadRequest, "invalid_role", fmt.Sprintf("%s is not a valid role", cuMembership.Role))
	}

	if !containsString(permissableRoles, role) {
		return errNoPermission
	}

	m, err := orgmembership.FindByUsernameAndOrgSlug(db, username, o.Slug)
	if err != nil {
		if err != sql.ErrNoRows {
			return errNotFound
		}
		if err = orgmembership.AddMember(db, o.ID, u.ID, role); err != nil {
			return err
		}
		env.Status = http.StatusCreated
	} else {
		if !containsString(permissableRoles, m.Role) {
			return errNoPermission
		}

		err = orgmembership.UpdateMember(db, o.ID, u.ID, role)
		if err != nil {
			if err == sql.ErrNoRows {
				return errNotFound
			}
			return err
		}
		env.Status = http.StatusOK
	}

	members, err := orgmembership.FindAllByOrgID(db, o.ID)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		members = []*orgmembership.OrgMember{}
	}

	env.Data = map[string]interface{}{
		"members": serializers.SerializeOrgMembers(members),
	}
	return nil
}

func containsString(strs []string, s string) bool {
	for _, str := range strs {
		if str == s {
			return true
		}
	}
	return false
}
