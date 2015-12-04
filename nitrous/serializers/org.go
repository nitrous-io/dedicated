package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/org"
)

type Org struct {
	ID       int64  `json:"id"`
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	MaxHosts int64  `json:"max_hosts,omitempty"`

	UsedHosts int `json:"used_hosts,omitempty"`

	Role string `json:"role,omitempty"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`

	MultiHost bool  `json:"multi_host"`
	Plan      *Plan `json:"plan,omitempty"`

	Edu bool `json:"edu"`

	BillingProfile *BillingProfile `json:"billing_profile,omitempty"`
}

func SerializeOrg(o *org.Org) *Org {
	if o == nil {
		return nil
	}
	serialized := &Org{
		ID:        o.ID,
		Slug:      o.Slug,
		Name:      o.Name,
		Email:     o.Email,
		MaxHosts:  o.MaxHosts.Int64,
		UsedHosts: o.UsedHosts,
		Edu:       o.Edu,
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}
	return serialized
}

func SerializeOrgWithRole(o *org.OrgWithRole) *Org {
	serialized := SerializeOrg(o.Org)
	serialized.Role = o.Role
	return serialized
}

func SerializeOrgsWithRoles(orgs []*org.OrgWithRole) []*Org {
	ret := make([]*Org, len(orgs))
	for i, o := range orgs {
		ret[i] = SerializeOrg(o.Org)
		ret[i].Role = o.Role
	}
	return ret
}
