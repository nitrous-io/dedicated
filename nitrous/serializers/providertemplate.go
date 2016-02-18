package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
)

type ProviderTemplate struct {
	ID        int64             `json:"id"`
	RegionID  int64             `json:"region_id"`
	PlanID    int64             `json:"plan_id"`
	Name      string            `json:"name"`
	Provider  string            `json:"provider"`
	Config    map[string]string `json:"config"`
	CreatedAt *time.Time        `json:"created_at"`
	UpdatedAt *time.Time        `json:"updated_at"`
}

func SerializeProviderTemplate(pt *providertemplate.ProviderTemplate) *ProviderTemplate {
	if pt == nil {
		return nil
	}

	spt := &ProviderTemplate{
		ID:        pt.ID,
		RegionID:  pt.RegionID,
		PlanID:    pt.PlanID,
		Name:      pt.Name,
		Provider:  pt.Provider,
		Config:    map[string]string{},
		CreatedAt: pt.CreatedAt,
		UpdatedAt: pt.UpdatedAt,
	}

	for key, value := range pt.Config.Map {
		if value.Valid == true {
			spt.Config[key] = value.String
		}
	}

	return spt
}

func SerializeProviderTemplates(pts []*providertemplate.ProviderTemplate) []*ProviderTemplate {
	res := make([]*ProviderTemplate, len(pts))

	for i, pt := range pts {
		res[i] = SerializeProviderTemplate(pt)
	}

	return res
}
