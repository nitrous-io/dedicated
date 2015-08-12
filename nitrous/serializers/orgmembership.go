package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
)

type OrgMember struct {
	ID       int64  `json:"id"`
	OrgID    int64  `json:"org_id"`
	OrgSlug  string `json:"org_slug"`
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`

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
		UserID:    o.UserID,
		Username:  o.Username,
		Name:      o.Name,
		Email:     o.Email,
		Role:      o.Role,
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
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
