package serializers

import "github.com/nitrous-io/nitrous/models/publickey"

type PublicKey struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	ContainerID int64  `json:"container_id"`
	Fingerprint string `json:"fingerprint"`
	Key         string `json:"key"`
}

func SerializePublicKey(k *publickey.PublicKey) *PublicKey {
	if k == nil {
		return nil
	}
	return &PublicKey{
		ID:          k.ID,
		UserID:      k.UserID,
		ContainerID: k.ContainerID,
		Fingerprint: k.Fingerprint,
		Key:         k.Key,
	}
}

func SerializePublicKeys(keys []*publickey.PublicKey) []*PublicKey {
	ret := make([]*PublicKey, len(keys))
	for i, k := range keys {
		ret[i] = SerializePublicKey(k)
	}
	return ret
}
