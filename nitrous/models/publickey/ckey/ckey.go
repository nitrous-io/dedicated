package ckey

import (
	"errors"
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/publickey"
)

var (
	ErrWrongKeyType = errors.New("set host_id on container key")
)

func Insert(db db.QueryReceiver, pk *publickey.PublicKey) error {
	if !pk.ContainerID.Valid || pk.ContainerID.Int64 == 0 {
		return publickey.ErrInvalidKey
	}
	if pk.HostID.Valid || pk.HostID.Int64 != 0 {
		return ErrWrongKeyType
	}
	return pk.RawInsert(db)
}

func FindAll(db db.QueryReceiver, userID int64, containerID int64) ([]*publickey.PublicKey, error) {
	keys := []*publickey.PublicKey{}
	err := db.Select(&keys, "SELECT * FROM public_keys WHERE user_id = $1 AND container_id = $2 ORDER BY created_at", userID, containerID)
	if err != nil {
		return []*publickey.PublicKey{}, err
	}
	return keys, nil
}

// Find queries for a Public Key scoped to a user and a container.
// identifier can be either an ID or a fingerprint.
func Find(db db.QueryReceiver, userID int64, containerID int64, identifier string) (*publickey.PublicKey, error) {
	keys := []publickey.PublicKey{}
	id, err := strconv.ParseInt(identifier, 10, 64)

	if err != nil {
		err = db.Select(&keys,
			"SELECT * FROM public_keys WHERE user_id = $1 AND container_id = $2 AND fingerprint = $3 ORDER BY created_at", userID, containerID, identifier)
	} else {
		err = db.Select(&keys,
			"SELECT * FROM public_keys WHERE user_id = $1 AND container_id = $2 AND id = $3 ORDER BY created_at", userID, containerID, id)
	}

	if len(keys) == 0 {
		return nil, publickey.ErrNotFound
	}
	if len(keys) > 1 {
		return nil, publickey.ErrDuplicateFingerprints
	}
	return &keys[0], err
}

func DestroyAll(db db.QueryReceiver, userID int64, containerID int64) error {
	_, err := db.Exec(`DELETE FROM public_keys WHERE user_id = $1 AND container_id = $2;`, userID, containerID)
	return err
}

func Destroy(db *db.Database, userID int64, containerID int64, id int64) error {
	_, err := db.Exec(`DELETE FROM public_keys WHERE user_id = $1 AND container_id = $2 AND id = $3;`, userID, containerID, id)
	return err
}
