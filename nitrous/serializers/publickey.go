package serializers

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/models/publickey"
)

type PublicKey struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	ContainerID *int64     `json:"container_id,omitempty"`
	HostID      *int64     `json:"host_id,omitempty"`
	Fingerprint string     `json:"fingerprint"`
	Key         string     `json:"key"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

func SerializePublicKey(k *publickey.PublicKey) *PublicKey {
	if k == nil {
		return nil
	}
	pk := &PublicKey{
		ID:          k.ID,
		UserID:      k.UserID,
		Fingerprint: k.Fingerprint,
		Key:         k.Key,
		CreatedAt:   k.CreatedAt,
		UpdatedAt:   k.UpdatedAt,
	}
	if k.ContainerID.Valid {
		pk.ContainerID = &k.ContainerID.Int64
	}
	if k.HostID.Valid {
		pk.HostID = &k.HostID.Int64
	}
	return pk
}

func SerializePublicKeys(keys []*publickey.PublicKey) []*PublicKey {
	ret := make([]*PublicKey, len(keys))
	for i, k := range keys {
		ret[i] = SerializePublicKey(k)
	}
	return ret
}
