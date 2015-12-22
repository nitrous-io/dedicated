package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
)

type OrgMember struct {
	ID       int64  `json:"id"`
	OrgID    int64  `json:"org_id"`
	OrgSlug  string `json:"org_slug,omitempty"`
	UserID   int64  `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Email    string `json:"email,omitempty"`
	Name     string `json:"name,omitempty"`
	Role     string `json:"role"`

	PendingEmail string `json:"pending_email,omitempty"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

func SerializeOrgMember(o *orgmembership.OrgMember) *OrgMember {
	if o == nil {
		return nil
	}
	serialized := &OrgMember{
		ID:        o.ID,
		OrgID:     o.OrgID,
		OrgSlug:   o.OrgSlug,
		Role:      o.Role,
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}

	if o.Name.Valid {
		serialized.Name = o.Name.String
	}
	if o.Username.Valid {
		serialized.Username = o.Username.String
	}
	if o.Email.Valid {
		serialized.Email = o.Email.String
	}
	if o.InviteEmail.Valid {
		serialized.PendingEmail = o.InviteEmail.String
	}
	if o.UserID.Valid {
		serialized.UserID = o.UserID.Int64
	}

	return serialized
}

func SerializeOrgMembers(oms []*orgmembership.OrgMember) []*OrgMember {
	ret := make([]*OrgMember, len(oms))
	for i, om := range oms {
		ret[i] = SerializeOrgMember(om)
	}
	return ret
}
