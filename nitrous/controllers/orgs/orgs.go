package orgs

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errBannedEmail      = httperror.New(http.StatusBadRequest, "email_banned", "this email address is banned")
	errNotFound         = httperror.New(http.StatusNotFound, "not_found", "requested resource could not be found")
	errNoPermission     = httperror.New(http.StatusForbidden, "permission_denied", "you do not have permission to edit members")
	errCannotRemoveSelf = httperror.New(http.StatusBadRequest, "invalid_action", "you cannot remove yourself from this organization")

	validations = map[string]error{
		"index_orgs_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
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

	err = db.WithTransaction(func(tx *sqlx.Tx) error {
		if err := o.Insert(tx); err != nil {
			if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
				if validationErr, ok := validations[e.Constraint]; ok {
					return validationErr
				}
			}
			return err
		}

		if err := billingprofile.Create(tx, user.ID, sql.NullInt64{o.ID, true}); err != nil {
			return err
		}

		if err := orgmembership.AddMember(tx, o.ID, user.ID, orgmembership.RoleOwner); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return err
	}

	bp, err := billingprofile.FindForOrg(db, o.ID)
	if err != nil {
		return err
	}

	// The creator of an org is always its owner.
	orgWithRole := &org.OrgWithRole{Org: o, Role: orgmembership.RoleOwner}
	ser := serializers.SerializeOrgWithRole(orgWithRole)
	ser.BillingProfile = serializers.SerializeBillingProfile(bp)

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"org": ser,
	}

	return nil
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	user := env.User

	orgs, err := org.FindAllByMemberUserID(db, user.ID)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		orgs = []*org.OrgWithRole{}
	}

	orgIDs := make([]int64, len(orgs))
	for i, o := range orgs {
		orgIDs[i] = o.ID
		count, err := o.UsedHostsCount(db)
		if err != nil {
			return err
		}
		o.UsedHosts = count
	}

	bps, err := billingprofile.FindForOrgs(db, orgIDs, user.ID)
	if err != nil {
		logrus.Errorf("Error fetching billing profiles for user %d err: %v", user.ID, err)
	}
	bpMap := make(map[int64]*billingprofile.BillingProfile)
	for _, bp := range bps {
		bpMap[bp.OrgID.Int64] = bp
	}

	plMap, err := plan.PlansForMultiHostOrgs(db, orgIDs)
	if err != nil {
		logrus.Errorf("Error fetching multi host plans for orgs %v err: %v", orgIDs, err)
	}

	ser := serializers.SerializeOrgsWithRoles(orgs)
	for _, o := range ser {
		if bp, ok := bpMap[o.ID]; ok {
			o.BillingProfile = serializers.SerializeBillingProfile(bp)
		}
		// if the org is part of multi host (otherwise it is an old org)
		if pl, ok := plMap[o.ID]; ok {
			o.Plan = serializers.SerializePlan(pl)
			o.MultiHost = true
		}
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"orgs": ser,
	}
	return nil
}

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	user := env.User
	slug := mux.Vars(r)["slug"]

	if slug == "" {
		return errNotFound
	}

	o, err := org.FindBySlug(db, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	count, err := o.UsedHostsCount(db)
	if err != nil {
		return err
	}
	o.UsedHosts = count

	mem, err := orgmembership.FindByUsernameAndOrgSlug(db, user.Username, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	ser := serializers.SerializeOrg(o)
	if mem.IsAdmin() {
		bp, err := billingprofile.FindForOrg(db, o.ID)
		if err != nil {
			return err
		}
		ser.BillingProfile = serializers.SerializeBillingProfile(bp)
	}

	plMap, err := plan.PlansForMultiHostOrgs(db, []int64{o.ID})
	if err != nil {
		logrus.Errorf("Error fetching multi host plans for org ID %d err: %v", o.ID, err)
	} else {
		if pl, ok := plMap[o.ID]; ok {
			ser.Plan = serializers.SerializePlan(pl)
			ser.MultiHost = true
		}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"org": ser,
	}

	return nil
}

func ShowMembers(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	user := env.User
	slug := mux.Vars(r)["slug"]

	if slug == "" {
		return errNotFound
	}

	o, err := org.FindBySlug(db, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	mem, err := orgmembership.FindByUsernameAndOrgSlug(db, user.Username, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	if mem.Role == orgmembership.RoleLimited {
		return errNotFound
	}

	members, err := orgmembership.FindAllByOrgID(db, o.ID)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		members = []*orgmembership.OrgMember{}
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"members": serializers.SerializeOrgMembers(members),
	}
	return nil
}

func PutMember(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	currentUser := env.User
	slug := strings.ToLower(mux.Vars(r)["slug"])
	username := strings.ToLower(mux.Vars(r)["username"])
	if username == "" {
		username = strings.ToLower(r.FormValue("username"))
	}

	if slug == "" || username == "" {
		return errNotFound
	}

	if currentUser.Username == username || currentUser.Email == username {
		return httperror.New(http.StatusBadRequest, "invalid_action", "you cannot edit yourself")
	}

	role := strings.ToLower(r.FormValue("role"))
	if role == "" {
		role = "member"
	}

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

	u, err := user.FindByUsernameOrEmail(db, username)
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

	m, err := orgmembership.FindByUsernameAndOrgSlug(db, u.Username, o.Slug)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
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

func RemoveMember(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	currentUser := env.User
	slug := mux.Vars(r)["slug"]
	username := mux.Vars(r)["username"]

	if slug == "" || username == "" {
		return errNotFound
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

	m, err := orgmembership.FindByUsernameAndOrgSlug(db, username, o.Slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	canRemove := true
	isSelf := cuMembership.UserID == u.ID
	if isSelf {
		canLeave := cuMembership.Role == orgmembership.RoleAdmin || cuMembership.Role == orgmembership.RoleMember
		if !canLeave {
			return errCannotRemoveSelf
		}
	} else {
		canRemove = containsString(permissableRoles, m.Role)
	}

	if !canRemove {
		return errNoPermission
	}

	err = orgmembership.RemoveMember(db, o.ID, u.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	if isSelf {
		env.Status = http.StatusOK
		env.Data = map[string]interface{}{
			"removed": true,
		}
		return nil
	}

	members, err := orgmembership.FindAllByOrgID(db, o.ID)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		members = []*orgmembership.OrgMember{}
	}

	env.Status = http.StatusOK
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
