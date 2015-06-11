package serializers

import "github.com/nitrous-io/dedicated/nitrous/models/plan"

type Plan struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	MaxContainers int64  `json:"max_containers"`
	Price         int64  `json:"price"`
	BasePrice     int64  `json:"base_price"`
	HourlyPrice   int64  `json:"hourly_price"`
	CPU           string `json:"cpu"`
	Memory        string `json:"memory"`
	Storage       string `json:"storage"`
	MostPopular   bool   `json:"most_popular"`
	AutoShutdown  bool   `json:"auto_shutdown"`
}

func SerializePlan(p *plan.Plan) *Plan {
	if p == nil {
		return nil
	}
	return &Plan{
		ID:            p.ID,
		Slug:          p.Slug,
		Name:          p.Name,
		MaxContainers: p.MaxContainers,
		Price:         p.Price,
		BasePrice:     p.BasePrice,
		HourlyPrice:   p.HourlyPrice(),
		CPU:           p.CPU,
		Memory:        p.Memory,
		Storage:       p.Storage,
		MostPopular:   p.MostPopular,
		AutoShutdown:  p.AutoShutdown,
	}
}

func SerializePlans(pls []*plan.Plan) []*Plan {
	ret := make([]*Plan, len(pls))
	for i, p := range pls {
		ret[i] = SerializePlan(p)
	}
	return ret
}
