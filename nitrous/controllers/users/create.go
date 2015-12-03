package users

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/controllers/github"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/oauthclient"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/cidrblacklist"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errInvalidOrgSlug       = httperror.New(http.StatusBadRequest, "invalid_org_slug", "org_slug is not allowed")
	errInvalidOrgPermission = httperror.New(http.StatusBadRequest, "invalid_org_permission", "org_permission must be `member`")
	errOrgNotFound          = httperror.New(http.StatusNotFound, "org_not_found", "org not found")

	invalidPromoCode  = httperror.New(http.StatusBadRequest, "invalid_promo_code", "promo code does not exist")
	invalidRemoteAddr = httperror.New(http.StatusBadRequest, "invalid_remote_addr", "remote_addr must be provided")

	bannedEmailAddress = httperror.New(http.StatusBadRequest, "email_banned", "this email address is banned")
	bannedPhone        = httperror.New(http.StatusBadRequest, "phone_banned", "this phone number is banned")

	phoneAlreadyAuthed = httperror.New(http.StatusBadRequest, "phone_authed", "phone number has already been verified")
)

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	email := strings.ToLower(r.FormValue("email"))
	username := strings.ToLower(r.FormValue("username"))
	password := r.FormValue("password")
	name := r.FormValue("name")
	promocode := r.FormValue("promo_code")
	invitecode := r.FormValue("invite_code")
	distinctID := r.FormValue("distinct_id")
	gitHubUserName := r.FormValue("github_username")

	u := &user.User{
		Email:    email,
		Username: username,
		Name:     name,
		Password: password,
	}

	err := u.Validate()
	if err != nil {
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	client, err := getOAuthClient(env, r)
	if err != nil {
		return err
	}
	if client != nil && !client.CreateUser {
		return errClientNotFound
	}

	// check email is not banned
	var banned bool
	err = db.Get(&banned, `SELECT COUNT(*) > 0 FROM users WHERE banned_at IS NOT NULL AND email = $1 LIMIT 1;`, u.Email)
	if err != nil {
		return err
	}
	if banned {
		return bannedEmailAddress
	}

	remoteAddr := env.RemoteAddr
	if client != nil {
		u.CreatedByClientID = sql.NullInt64{client.ID, true}
		if remoteAddr = r.FormValue("remote_addr"); remoteAddr == "" {
			return invalidRemoteAddr
		}
	}

	// if user is in CIDR blacklist, they are suspicious
	if err := cidrblacklist.Contains(remoteAddr); err != nil {
		u.Suspicious = sql.NullString{String: err.Error(), Valid: true}
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}

	err = u.Insert(tx)
	if err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}

	if client != nil {
		if err := addToOrg(tx, u, r, client); err != nil {
			tx.Rollback()
			return err
		}
	}

	if gitHubUserName != "" {
		if err := github.UpdateGitHubInfoFromRedis(tx, u.ID, gitHubUserName); err != nil {
			tx.Rollback()
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	if promocode != "" {
		// TODO: insert a claim
	}

	confirmable := client != nil
	if !confirmable && invitecode != "" {
		err = db.Get(&confirmable, `SELECT COUNT(*) = 1 FROM container_collabs WHERE invite_email = $1 AND invite_code = $2 LIMIT 1`, u.Email, invitecode)
		if err != nil {
			return err
		}
	}

	if confirmable {
		_, err = db.Exec("UPDATE users SET confirmed_at = now() WHERE id = $1", u.ID)
		if err != nil {
			return err
		}
		_, err = db.Exec("UPDATE container_collabs SET user_id = $1, invite_email = NULL, invite_code = NULL WHERE invite_email = $2", u.ID, u.Email)
		if err != nil {
			return err
		}
		notifySharedContainers(env, u)
	} else {
		err = u.SendConfirmationEmail(db, true)
		if err != nil {
			return err
		}
	}

	env.Tracker.CreateProfile(u, distinctID)

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"user": serializers.SerializeUser(u),
	}
	env.User = u

	return nil
}

func addToOrg(tx *sqlx.Tx, u *user.User, r *http.Request, client *oauthclient.Client) error {
	orgSlug := r.FormValue("org_slug")
	if orgSlug == "" {
		return nil
	}

	orgPermission := r.FormValue("org_permission")
	if !(orgPermission == orgmembership.RoleMember || orgPermission == orgmembership.RoleLimited) {
		return errInvalidOrgPermission
	}

	if !client.AllowedOrgSlug(orgSlug) {
		return errInvalidOrgSlug
	}

	var orgID int64
	err := tx.Get(&orgID, "SELECT id FROM orgs WHERE slug = $1;", orgSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errOrgNotFound
		}
		return err
	}

	_, err = tx.Exec("INSERT INTO org_memberships (user_id, org_id, role) VALUES ($1, $2, $3)", u.ID, orgID, orgPermission)
	return err
}
