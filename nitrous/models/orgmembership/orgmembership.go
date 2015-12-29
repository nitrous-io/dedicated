package orgmembership

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

var ErrDoesNotExist = errors.New("org membership does not exist")

const (
	RoleOwner   = "owner"
	RoleAdmin   = "admin"
	RoleMember  = "member"
	RoleLimited = "limited"
)

var ValidRoles = map[string][]string{
	RoleOwner:   {RoleOwner, RoleAdmin, RoleMember, RoleLimited},
	RoleAdmin:   {RoleAdmin, RoleMember, RoleLimited},
	RoleMember:  {},
	RoleLimited: {},
}

type InvalidRoleError struct {
	Role string
}

func (e InvalidRoleError) Error() string {
	return fmt.Sprintf("invalid role: %s", e.Role)
}

type OrgMembership struct {
	ID     int64         `db:"id"`
	OrgID  int64         `db:"org_id"`
	UserID sql.NullInt64 `db:"user_id"`
	Role   string        `db:"role"`

	EmailSentAt pq.NullTime `db:"email_sent_at"`

	InviteEmail sql.NullString `db:"invite_email"`
	InviteCode  sql.NullString `db:"invite_code"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func (o *OrgMembership) IsAdmin() bool {
	return o.Role == RoleOwner || o.Role == RoleAdmin
}

func (o *OrgMembership) SnapshotPublishAllowed() bool {
	return o.Role == RoleOwner || o.Role == RoleAdmin || o.Role == RoleMember
}

func (o *OrgMembership) SnapshotDeleteAllowed() bool {
	return o.Role == RoleOwner || o.Role == RoleAdmin
}

func (o *OrgMembership) SnapshotReadAllowed() bool {
	return o.Role == RoleOwner || o.Role == RoleAdmin || o.Role == RoleMember
}

type OrgMember struct {
	OrgMembership

	OrgSlug  string         `db:"org_slug"`
	Username sql.NullString `db:"username"`
	Email    sql.NullString `db:"email"`
	Name     sql.NullString `db:"name"`
}

func FindAllByOrgID(d db.QueryReceiver, id int64) ([]*OrgMember, error) {
	members := []*OrgMember{}
	err := d.Select(&members, `
		SELECT org_memberships.*, orgs.slug as org_slug, users.username, users.email, users.name
		FROM org_memberships
		LEFT JOIN users ON org_memberships.user_id = users.id
		INNER JOIN orgs ON org_memberships.org_id = orgs.id
		WHERE org_memberships.org_id = $1
		ORDER BY org_memberships.id;
	`, id)
	if err != nil {
		return nil, err
	}
	return members, nil
}

// Identifier can be the userid, username or invite email
func FindByIdentifierAndOrgSlug(d db.QueryReceiver, identifier, slug string) (*OrgMembership, error) {
	member := &OrgMembership{}

	id, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		err = d.Get(member, `
		  SELECT org_memberships.*
		  FROM org_memberships
		  LEFT JOIN users ON org_memberships.user_id = users.id
		  INNER JOIN orgs ON org_memberships.org_id = orgs.id
		  WHERE (
		    (users.username = $1 OR invite_email = $1)
			  AND orgs.slug = $2
		  )
		  ORDER BY org_memberships.id
	  `, identifier, slug)
	} else {
		err = d.Get(member, `
		  SELECT org_memberships.*
		  FROM org_memberships
		  INNER JOIN orgs ON org_memberships.org_id = orgs.id
		  WHERE (
		    user_id = $1
			  AND orgs.slug = $2
		  )
		  ORDER BY org_memberships.id
	  `, id, slug)
	}
	if err != nil {
		return nil, err
	}
	return member, nil
}

func FindByUserIDAndOrgID(d db.QueryReceiver, userID, orgID int64) (*OrgMembership, error) {
	member := &OrgMembership{}
	err := d.Get(member, `
		SELECT org_memberships.*
		FROM org_memberships
		WHERE user_id = $1
		AND org_id = $2`, userID, orgID)
	if err != nil {
		return nil, err
	}
	return member, nil
}

func AddMember(d db.QueryReceiver, orgID int64, identifier, role string) (*OrgMembership, error) {
	if _, ok := ValidRoles[role]; !ok {
		return nil, InvalidRoleError{Role: role}
	}

	var s *sqlx.NamedStmt
	om := &OrgMembership{
		OrgID: orgID,
		Role:  role,
	}

	id, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		token, tErr := securerandom.Alphanumeric(20)
		if tErr != nil {
			return nil, err
		}
		om.InviteEmail = sql.NullString{identifier, true}
		om.InviteCode = sql.NullString{token, true}
		s, err = d.PrepareNamed(`
		  INSERT INTO org_memberships
		  (org_id, invite_email, invite_code, role, email_sent_at)
		  VALUES (:org_id, :invite_email, :invite_code, :role, now())
		  RETURNING *;`)
	} else {
		om.UserID = sql.NullInt64{id, true}
		s, err = d.PrepareNamed(`
		  INSERT INTO org_memberships
		  (org_id, user_id, role)
		  VALUES (:org_id, :user_id, :role)
		  RETURNING *;`)
	}
	if err != nil {
		return nil, err
	}
	_, err = s.Exec(om)
	return om, err
}

func UpdateMember(d db.QueryReceiver, orgID int64, identifier, role string) error {
	if _, ok := ValidRoles[role]; !ok {
		return InvalidRoleError{Role: role}
	}

	var r sql.Result

	id, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		r, err = d.Exec(`
		  UPDATE org_memberships
		  SET role = $1, updated_at = now()
		  WHERE org_id = $2 AND invite_email = $3;
	`, role, orgID, identifier)
	} else {
		r, err = d.Exec(`
		  UPDATE org_memberships
		  SET role = $1, updated_at = now()
		  WHERE org_id = $2 AND user_id = $3;
	`, role, orgID, id)

	}
	if err != nil {
		return err
	}

	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func RemoveMember(d db.QueryReceiver, orgID int64, identifier string) error {
	var r sql.Result

	id, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		r, err = d.Exec(`
		  DELETE FROM org_memberships
		  WHERE org_id = $1 AND invite_email = $2
	`, orgID, identifier)
	} else {
		r, err = d.Exec(`
		  DELETE FROM org_memberships
		  WHERE org_id = $1 AND user_id = $2
	`, orgID, id)
	}
	if err != nil {
		return err
	}

	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func PromotePendingMemberToUser(d db.QueryReceiver, email, invitecode string, userID int64) (*OrgMembership, error) {
	var exists bool
	err := d.Get(&exists, `
	        SELECT COUNT(*) = 1
		FROM org_memberships
		WHERE invite_email = $1
		AND invite_code = $2
		LIMIT 1`, email, invitecode)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, ErrDoesNotExist
	}

	om := &OrgMembership{
		UserID:      sql.NullInt64{userID, true},
		InviteEmail: sql.NullString{email, true},
	}
	nstmt, err := d.PrepareNamed(
		`UPDATE org_memberships
		SET (user_id, invite_email, invite_code)
		= (:user_id, NULL, NULL)
		WHERE invite_email = :invite_email
		RETURNING *;`)
	err = nstmt.QueryRowx(om).StructScan(om)
	return om, err
}
