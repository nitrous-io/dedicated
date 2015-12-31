package orgs

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/blacklist"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/mailer"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errBannedEmail                   = httperror.New(http.StatusBadRequest, "email_banned", "this email address is banned")
	errNotFound                      = httperror.New(http.StatusNotFound, "not_found", "requested resource could not be found")
	errUserNotFound                  = httperror.New(http.StatusNotFound, "user_not_found", "the specified user could not be found")
	errEditNotPermitted              = httperror.New(http.StatusForbidden, "permission_denied", "you do not have permission to edit this member")
	errRemoveNotPermitted            = httperror.New(http.StatusForbidden, "permission_denied", "you do not have permission to remove this member")
	errEditSelfForbidden             = httperror.New(http.StatusBadRequest, "invalid_action", "you cannot edit yourself")
	errAddMemberFailed               = httperror.New(http.StatusBadRequest, "invalid_action", "failed to add member to organization! Support has been notified")
	errCannotRemoveSelf              = httperror.New(http.StatusBadRequest, "invalid_action", "you cannot remove yourself from this organization")
	errNotEduWhitelisted             = httperror.New(http.StatusForbidden, "not_edu_whitelisted", "Please contact Nitrous to enable education support")
	errCannotLeaveWithExistingHosts  = httperror.New(http.StatusForbidden, "forbidden", "you cannot leave this organization because you still have existing workstations")
	errCannotRemoveWithExistingHosts = httperror.New(http.StatusForbidden, "forbidden", "you cannot remove this member because they still have existing workstations")

	validations = map[string]error{
		"index_orgs_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
	Mailer mailer.Mailer
)

func init() {
	Mailer = mailer.NewSendGridMailer(envvars.SendgridUsername, envvars.SendgridPassword)
}

func Create(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	user := env.User

	slug := strings.ToLower(r.FormValue("slug"))
	name := r.FormValue("name")
	email := strings.ToLower(r.FormValue("email"))
	intent := r.FormValue("intent")

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

	// check email qualifies for edu
	var edu bool
	err = db.Get(&edu, `SELECT COUNT(*) > 0 FROM edu_emails_whitelist WHERE email = $1 LIMIT 1;`, o.Email)
	if err != nil {
		return err
	}
	o.Edu = edu

	if intent == "edu" && !o.Edu {
		return errNotEduWhitelisted
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

		if _, err := orgmembership.AddMember(tx, o.ID, strconv.FormatInt(user.ID, 10), orgmembership.RoleOwner); err != nil {
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

	mem, err := orgmembership.FindByIdentifierAndOrgSlug(db, user.Username, slug)
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

	mem, err := orgmembership.FindByIdentifierAndOrgSlug(db, user.Username, slug)
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
		return err
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
	identifier := strings.ToLower(mux.Vars(r)["identifier"])

	if slug == "" || identifier == "" {
		return errNotFound
	}

	if env.User.Username == identifier || env.User.Email == identifier {
		return errEditSelfForbidden
	}

	role := strings.ToLower(r.FormValue("role"))
	if role == "" {
		role = "member"
	}

	if _, ok := orgmembership.ValidRoles[role]; !ok {
		return httperror.New(http.StatusBadRequest, "invalid_role", fmt.Sprintf("%s is not a valid role", role))
	}

	o, err := org.FindBySlug(env.DB, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	u, err := user.FindByUsernameOrEmail(db, identifier)
	if err != nil {
		if err == sql.ErrNoRows {
			// We will send an invite email if the email provided does not exist in the user database.
			if !strings.Contains(identifier, "@") || blacklist.Check(identifier) {
				return errUserNotFound
			}
		} else {
			return err
		}
	}

	permissibleRoles, err := getPermissibleRoles(env, strconv.FormatInt(currentUser.ID, 10), o.Slug)
	if err != nil {
		return err
	}

	if !containsString(permissibleRoles, role) {
		return errEditNotPermitted
	}

	if u != nil {
		identifier = strconv.FormatInt(u.ID, 10)
	}

	if err := handleUpdateOrAddMember(env, o, permissibleRoles, identifier, role); err != nil {
		return err
	}

	return serializeOrgMembersData(env, o)
}

func getPermissibleRoles(env *middleware.Env, identifier, orgSlug string) ([]string, error) {
	cuMembership, err := orgmembership.FindByIdentifierAndOrgSlug(env.DB, identifier, orgSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errEditNotPermitted
		}
		return nil, err
	}

	permissibleRoles, ok := orgmembership.ValidRoles[cuMembership.Role]
	if !ok {
		return nil, httperror.New(http.StatusBadRequest, "invalid_role", fmt.Sprintf("%s is not a valid role", cuMembership.Role))
	}

	return permissibleRoles, nil
}

func handleUpdateOrAddMember(env *middleware.Env, o *org.Org, permissibleRoles []string, identifier, role string) error {
	m, err := orgmembership.FindByIdentifierAndOrgSlug(env.DB, identifier, o.Slug)
	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		err := env.DB.WithTransaction(func(tx *sqlx.Tx) error {
			om, err := orgmembership.AddMember(tx, o.ID, identifier, role)
			if err != nil {
				return err
			}
			if om.InviteEmail.Valid && om.InviteCode.Valid {
				if err := inviteUser(tx, o, om.InviteCode.String, identifier, env.User); err != nil {
					logrus.Errorf("Send org invite email failed: orgID - %d, initiator userID - %d, invited member - %s. Got err: %v", o.ID, env.User.ID, identifier, err)
					return errAddMemberFailed
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		env.Status = http.StatusCreated
	} else {
		if !containsString(permissibleRoles, m.Role) {
			return errEditNotPermitted
		}
		if err := orgmembership.UpdateMember(env.DB, o.ID, identifier, role); err != nil {
			if err == sql.ErrNoRows {
				return errNotFound
			}
			return err
		}
		env.Status = http.StatusOK
	}
	return nil
}

func serializeOrgMembersData(env *middleware.Env, o *org.Org) error {
	members, err := orgmembership.FindAllByOrgID(env.DB, o.ID)
	if err != nil {
		return err
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
	identifier := mux.Vars(r)["identifier"]
	byInviteEmail := strings.Contains(identifier, "@")

	if slug == "" || identifier == "" {
		return errNotFound
	}

	o, err := org.FindBySlug(db, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	var u *user.User
	if !byInviteEmail {
		u, err = user.FindByUsername(db, identifier)
		if err != nil {
			if err == sql.ErrNoRows {
				return errNotFound
			}
		}
	}

	cuMembership, err := orgmembership.FindByIdentifierAndOrgSlug(db, currentUser.Username, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errRemoveNotPermitted
		}
	}

	permissableRoles, ok := orgmembership.ValidRoles[cuMembership.Role]
	if !ok {
		return httperror.New(http.StatusBadRequest, "invalid_role", fmt.Sprintf("%s is not a valid role", cuMembership.Role))
	}

	m, err := orgmembership.FindByIdentifierAndOrgSlug(db, identifier, o.Slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotFound
		}
		return err
	}

	canRemove := true
	isSelf := false
	if cuMembership.UserID.Valid && u != nil {
		isSelf = cuMembership.UserID.Int64 == u.ID
	}
	if isSelf {
		canLeave := cuMembership.Role == orgmembership.RoleAdmin || cuMembership.Role == orgmembership.RoleMember
		if !canLeave {
			return errCannotRemoveSelf
		}
	} else {
		canRemove = containsString(permissableRoles, m.Role)
	}

	if !canRemove {
		return errRemoveNotPermitted
	}

	// User must not have any more valid hosts under this org.
	hosts, err := host.FindAllByUserAndOrg(db, u.ID, o.ID)
	if err != nil {
		return err
	}
	if len(hosts) > 0 {
		if isSelf {
			return errCannotLeaveWithExistingHosts
		}
		return errCannotRemoveWithExistingHosts
	}

	if u != nil {
		identifier = strconv.FormatInt(u.ID, 10)
	}

	if err := orgmembership.RemoveMember(db, o.ID, identifier); err != nil {
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
		return err
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
