package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
)

type Snapshot struct {
	ID          int64  `json:"id"`
	ImageID     int64  `json:"image_id"`
	UserID      int64  `json:"user_id"`
	OrgID       int64  `json:"org_id"`
	DisplayName string `json:"display_name"`
	Stack       string `json:"stack"`

	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func SerializeSnapshot(s *snapshot.Snapshot) *Snapshot {
	if s == nil {
		return nil
	}
	serialized := &Snapshot{
		ID:          s.ID,
		ImageID:     s.ImageID,
		UserID:      s.UserID,
		DisplayName: s.DisplayName,
		Stack:       s.Stack(),
	}

	if s.OrgID.Valid {
		serialized.OrgID = s.OrgID.Int64
	}
	if s.CreatedAt.Valid {
		serialized.CreatedAt = &s.CreatedAt.Time
	}
	if s.UpdatedAt.Valid {
		serialized.UpdatedAt = &s.UpdatedAt.Time
	}

	return serialized
}

func SerializeSnapshots(snapshots []*snapshot.Snapshot) []*Snapshot {
	ret := make([]*Snapshot, len(snapshots))
	for i, s := range snapshots {
		ret[i] = SerializeSnapshot(s)
	}
	return ret
}
