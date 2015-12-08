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
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Size        int64  `json:"size,omitempty"`
	Stack       string `json:"stack"`

	OwnerUsername string `json:"owner_username,omitempty"`
	OwnerName     string `json:"owner_name,omitempty"`
	OwnerEmail    string `json:"owner_email,omitempty"`

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
		Slug:        s.Slug,
		DisplayName: s.DisplayName,
		Stack:       s.Stack(),
	}

	if s.OrgID.Valid {
		serialized.OrgID = s.OrgID.Int64
	}
	if s.Size.Valid {
		serialized.Size = s.Size.Int64
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

func SerializeSnapshotWithOwner(s *snapshot.SnapshotWithOwner) *Snapshot {
	if s == nil || s.Snapshot == nil {
		return nil
	}
	serialized := SerializeSnapshot(s.Snapshot)

	if s.OwnerUsername.Valid {
		serialized.OwnerUsername = s.OwnerUsername.String
	}
	if s.OwnerName.Valid {
		serialized.OwnerName = s.OwnerName.String
	}
	if s.OwnerEmail.Valid {
		serialized.OwnerEmail = s.OwnerEmail.String
	}

	return serialized
}

func SerializeSnapshotsWithOwner(snapshots []*snapshot.SnapshotWithOwner) []*Snapshot {
	ret := make([]*Snapshot, len(snapshots))
	for i, s := range snapshots {
		ret[i] = SerializeSnapshotWithOwner(s)
	}
	return ret
}
