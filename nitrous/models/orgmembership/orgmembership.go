package orgmembership

import (
	"fmt"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

var ValidRoles = map[string]struct{}{
	"member": {},
	"owner":  {},
	"admin":  {},
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

type OrgMember struct {
	OrgMembership

	Username string `db:"username"`
	Email    string `db:"email"`
	Name     string `db:"name"`
}

func FindAllByOrgId(d db.QueryReceiver, id int64) ([]*OrgMember, error) {
	members := []*OrgMember{}
	err := d.Select(&members, `
		SELECT org_memberships.*, users.username, users.email, users.name
		FROM org_memberships
		INNER JOIN users ON org_memberships.user_id = users.id
		WHERE org_memberships.org_id = $1
		ORDER BY org_memberships.id;
	`, id)
	if err != nil {
		return nil, err
	}
	return members, nil
}
