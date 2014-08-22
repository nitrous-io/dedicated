package serializers

import "github.com/nitrous-io/nitrous/models/plan"

type Plan struct {
	ID            int64  `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	MaxContainers int64  `json:"max_containers"`
	Price         int64  `json:"price"`
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
	}
}

func SerializePlans(pls []*plan.Plan) []*Plan {
	ret := make([]*Plan, len(pls))
	for i, p := range pls {
		ret[i] = SerializePlan(p)
	}
	return ret
}
