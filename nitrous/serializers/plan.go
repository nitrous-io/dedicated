package serializers

import "github.com/nitrous-io/dedicated/nitrous/models/plan"

type Plan struct {
	ID                int64 `json:"id"`
	BasePlanID        int64 `json:"base_plan_id,omitempty"`
	HostingModePlanID int64 `json:"hosting_mode_plan_id,omitempty"`

	Slug         string `json:"slug"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Type         string `json:"type"`
	Price        int64  `json:"price"`
	BasePrice    int64  `json:"base_price"`
	HourlyPrice  int64  `json:"hourly_price"`
	CPU          string `json:"cpu"`
	Memory       string `json:"memory"`
	Storage      string `json:"storage"`
	MostPopular  bool   `json:"most_popular"`
	Subscription bool   `json:"subscription"`

	Hidden bool `json:"hidden"`

	TrialPeriodDays int64 `json:"trial_period_days"`

	AutoShutdown    bool  `json:"auto_shutdown"`
	MaxContainers   int64 `json:"max_containers"`
	MaxSnapshots    int64 `json:"max_snapshots"`
	MaxHosts        int64 `json:"max_hosts,omitempty"`
	CleanPreview    bool  `json:"clean_preview"`
	DesktopApp      bool  `json:"desktop_app"`
	SSH             bool  `json:"ssh"`
	CustomDomain    bool  `json:"custom_domain"`
	CustomTemplates bool  `json:"custom_templates"`
	Org             bool  `json:"org"`
}

func SerializePlan(p *plan.Plan) *Plan {
	if p == nil {
		return nil
	}
	serialized := &Plan{
		ID:              p.ID,
		Slug:            p.Slug,
		Name:            p.Name,
		Description:     p.Description,
		Type:            p.Type,
		Price:           p.Price,
		BasePrice:       p.BasePrice,
		HourlyPrice:     p.HourlyPrice(),
		CPU:             p.CPU,
		Memory:          p.Memory,
		Storage:         p.Storage,
		MostPopular:     p.MostPopular,
		Subscription:    p.StripePlanID.Valid,
		TrialPeriodDays: p.TrialPeriodDays,
		Hidden:          p.Hidden,
		AutoShutdown:    p.AutoShutdown,
		MaxContainers:   p.MaxContainers,
		MaxSnapshots:    p.MaxSnapshots,
		MaxHosts:        p.MaxHosts.Int64,
		CleanPreview:    p.CleanPreview,
		DesktopApp:      p.DesktopApp,
		SSH:             p.SSH,
		CustomDomain:    p.CustomDomain,
		CustomTemplates: p.CustomTemplates,
		Org:             p.Org,
	}
	if p.BasePlanID.Valid {
		serialized.BasePlanID = p.BasePlanID.Int64
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
