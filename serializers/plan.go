package serializers

import "github.com/nitrous-io/nitrous/models/plan"

type Plan struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func SerializePlan(p *plan.Plan) *Plan {
	if p == nil {
		return nil
	}
	return &Plan{
		ID:   p.ID,
		Name: p.Name,
	}
}
