package serializers

import "github.com/nitrous-io/nitrous/models/region"

type Region struct {
	ID   int64  `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func SerializeRegion(r *region.Region) *Region {
	if r == nil {
		return nil
	}
	return &Region{
		ID:   r.ID,
		Slug: r.Slug,
		Name: r.Name,
	}
}

func SerializeRegions(rs []*region.Region) []*Region {
	ret := make([]*Region, len(rs))
	for i, r := range rs {
		ret[i] = SerializeRegion(r)
	}
	return ret
}
