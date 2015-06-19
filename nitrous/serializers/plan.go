package serializers

import "github.com/nitrous-io/dedicated/nitrous/models/plan"

type Plan struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Price       int64  `json:"price"`
	BasePrice   int64  `json:"base_price"`
	HourlyPrice int64  `json:"hourly_price"`
	CPU         string `json:"cpu"`
	Memory      string `json:"memory"`
	Storage     string `json:"storage"`
	MostPopular bool   `json:"most_popular"`

	MaxContainers   int64 `json:"max_containers"`
	MaxSnapshots    int64 `json:"max_snapshots"`
	CleanPreview    bool  `json:"clean_preview"`
	DesktopApp      bool  `json:"desktop_app"`
	SSH             bool  `json:"ssh"`
	CustomDomain    bool  `json:"custom_domain"`
	CustomTemplates bool  `json:"custom_templates"`
}

func SerializePlan(p *plan.Plan) *Plan {
	if p == nil {
		return nil
	}
	return &Plan{
		ID:              p.ID,
		Slug:            p.Slug,
		Name:            p.Name,
		Price:           p.Price,
		BasePrice:       p.BasePrice,
		HourlyPrice:     p.HourlyPrice(),
		CPU:             p.CPU,
		Memory:          p.Memory,
		Storage:         p.Storage,
		MostPopular:     p.MostPopular,
		MaxContainers:   p.MaxContainers,
		MaxSnapshots:    p.MaxSnapshots,
		CleanPreview:    p.CleanPreview,
		DesktopApp:      p.DesktopApp,
		SSH:             p.SSH,
		CustomDomain:    p.CustomDomain,
		CustomTemplates: p.CustomTemplates,
	}
}

func SerializePlans(pls []*plan.Plan) []*Plan {
	ret := make([]*Plan, len(pls))
	for i, p := range pls {
		ret[i] = SerializePlan(p)
	}
	return ret
}
