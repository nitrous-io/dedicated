package serializers

import "github.com/nitrous-io/dedicated/nitrous/models/plan"

type Plan struct {
	ID                int64 `json:"id"`
	HostingModePlanID int64 `json:"hosting_mode_plan_id,omitempty"`

	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Price        int64  `json:"price"`
	CPU          string `json:"cpu"`
	Memory       string `json:"memory"`
	Storage      string `json:"storage"`
	Subscription bool   `json:"subscription"`

	Hidden bool `json:"hidden"`

	TrialPeriodDays int64 `json:"trial_period_days"`

	AutoShutdown     bool  `json:"auto_shutdown"`
	MaxContainers    int64 `json:"max_containers"`
	MaxHosts         int64 `json:"max_hosts,omitempty"`
	MaxCollaborators int64 `json:"max_collaborators,omitempty"`

	Org bool `json:"org"`
	Edu bool `json:"edu"`
}

func SerializePlan(p *plan.Plan) *Plan {
	if p == nil {
		return nil
	}
	serialized := &Plan{
		ID:               p.ID,
		Slug:             p.Slug,
		Name:             p.Name,
		Description:      p.Description,
		Price:            p.Price,
		CPU:              p.CPU,
		Memory:           p.Memory,
		Storage:          p.Storage,
		Subscription:     p.StripePlanID.Valid,
		TrialPeriodDays:  p.TrialPeriodDays,
		Hidden:           p.Hidden,
		AutoShutdown:     p.AutoShutdown,
		MaxContainers:    p.MaxContainers,
		MaxHosts:         p.MaxHosts.Int64,
		MaxCollaborators: p.MaxCollaborators.Int64,
		Org:              p.Org,
		Edu:              p.Edu,
	}
	if p.HostingModePlanID.Valid {
		serialized.HostingModePlanID = p.HostingModePlanID.Int64
	}
	return serialized
}

func SerializePlans(pls []*plan.Plan) []*Plan {
	ret := make([]*Plan, len(pls))
	for i, p := range pls {
		ret[i] = SerializePlan(p)
	}
	return ret
}
