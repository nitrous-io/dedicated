package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/userkey"
)

type UserKey struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	ContainerID int64      `json:"container_id"`
	Fingerprint string     `json:"fingerprint"`
	Key         string     `json:"key"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

func SerializeUserKey(k *userkey.UserKey) *UserKey {
	if k == nil {
		return nil
	}
	return &UserKey{
		ID:          k.ID,
		UserID:      k.UserID,
		ContainerID: k.ContainerID,
		Fingerprint: k.Fingerprint,
		Key:         k.Key,
		CreatedAt:   k.CreatedAt,
		UpdatedAt:   k.UpdatedAt,
	}
}

func SerializeUserKeys(keys []*userkey.UserKey) []*UserKey {
	ret := make([]*UserKey, len(keys))
	for i, k := range keys {
		ret[i] = SerializeUserKey(k)
	}
	return ret
}
