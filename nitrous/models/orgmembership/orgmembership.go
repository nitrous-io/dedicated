package orgmembership

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

var ValidRoles = map[string][]string{
	RoleOwner:  {RoleOwner, RoleAdmin, RoleMember},
	RoleAdmin:  {RoleAdmin, RoleMember},
	RoleMember: {},
}

type InvalidRoleError struct {
	Role string
}

func (e InvalidRoleError) Error() string {
	return fmt.Sprintf("invalid role: %s", e.Role)
}

type OrgMembership struct {
	ID     int64  `db:"id"`
	OrgID  int64  `db:"org_id"`
	UserID int64  `db:"user_id"`
	Role   string `db:"role"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func (o *OrgMembership) IsAdmin() bool {
	return o.Role == "owner" || o.Role == "admin"
}

type OrgMember struct {
	OrgMembership

	OrgSlug  string `db:"org_slug"`
	Username string `db:"username"`
	Email    string `db:"email"`
	Name     string `db:"name"`
}

func FindAllByOrgID(d db.QueryReceiver, id int64) ([]*OrgMember, error) {
	members := []*OrgMember{}
	err := d.Select(&members, `
		SELECT org_memberships.*, orgs.slug as org_slug, users.username, users.email, users.name
		FROM org_memberships
		INNER JOIN users ON org_memberships.user_id = users.id
		INNER JOIN orgs ON org_memberships.org_id = orgs.id
		WHERE org_memberships.org_id = $1
		ORDER BY org_memberships.id;
	`, id)
	if err != nil {
		return nil, err
	}
	return members, nil
}

func FindByUsernameAndOrgSlug(d db.QueryReceiver, username, slug string) (*OrgMembership, error) {
	member := &OrgMembership{}
	err := d.Get(member, `
		SELECT org_memberships.*
		FROM org_memberships
		INNER JOIN users ON org_memberships.user_id = users.id
		INNER JOIN orgs ON org_memberships.org_id = orgs.id
		WHERE users.username = $1 AND orgs.slug = $2
		ORDER BY org_memberships.id
	`, username, slug)
	if err != nil {
		return nil, err
	}
	return member, nil
}

func AddMember(d db.QueryReceiver, orgID, userID int64, role string) error {
	if _, ok := ValidRoles[role]; !ok {
		return InvalidRoleError{Role: role}
	}
	om := &OrgMembership{
		OrgID:  orgID,
		UserID: userID,
		Role:   role,
	}
	s, err := d.PrepareNamed(`
		INSERT INTO org_memberships
		(org_id, user_id, role)
		VALUES (:org_id, :user_id, :role)
	`)
	if err != nil {
		return err
	}
	_, err = s.Exec(om)
	return err
}

func UpdateMember(d db.QueryReceiver, orgID, userID int64, role string) error {
	if _, ok := ValidRoles[role]; !ok {
		return InvalidRoleError{Role: role}
	}
	r, err := d.Exec(`
		UPDATE org_memberships
		SET role = $1, updated_at = now()
		WHERE org_id = $2 AND user_id = $3
	`, role, orgID, userID)
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

func RemoveMember(d db.QueryReceiver, orgID, userID int64) error {
	r, err := d.Exec(`
		DELETE FROM org_memberships
		WHERE org_id = $1 AND user_id = $2
	`, orgID, userID)
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
