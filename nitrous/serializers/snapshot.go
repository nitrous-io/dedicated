package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
)

type Snapshot struct {
	ID        int64      `json:"id,omitempty"`
	Size      int64      `json:"size"`
	Url       string     `json:"url"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func SerializeSnapshot(s *snapshot.Snapshot) *Snapshot {
	if s == nil {
		return nil
	}
	return &Snapshot{
		ID:        s.ID,
		Size:      s.Size,
		Url:       s.Url,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func SerializeSnapshots(snaps []*snapshot.Snapshot) []*Snapshot {
	ret := make([]*Snapshot, len(snaps))
	for i, s := range snaps {
		ret[i] = SerializeSnapshot(s)
	}
	return ret
}
