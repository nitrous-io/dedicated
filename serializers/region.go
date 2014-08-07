package serializers

import "github.com/nitrous-io/nitrous/models/region"

type Region struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func SerializeRegion(r *region.Region) *Region {
	if r == nil {
		return nil
	}
	return &Region{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
	}
}
