package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/org"
)

type Org struct {
	ID    int64  `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Email string `json:"email"`

	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`

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
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}
	return serialized
}

func SerializeOrgs(orgs []*org.Org) []*Org {
	ret := make([]*Org, len(orgs))
	for i, o := range orgs {
		ret[i] = SerializeOrg(o)
	}
	return ret
}
