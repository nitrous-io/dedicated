package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/volume"
)

type Volume struct {
	ID        int64      `json:"id,omitempty"`
	Name      string     `json:"name,omitempty"`
	Slug      string     `json:"slug"`
	HostID    int64      `json:"host_id,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func SerializeVolume(v *volume.Volume) *Volume {
	if v == nil {
		return nil
	}
	return &Volume{
		ID:        v.ID,
		Name:      v.Name,
		Slug:      v.Slug,
		HostID:    v.HostID,
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
	}
}

func SerializeVolumes(vols []*volume.Volume) []*Volume {
	ret := make([]*Volume, len(vols))
	for i, v := range vols {
		ret[i] = SerializeVolume(v)
	}
	return ret
}
