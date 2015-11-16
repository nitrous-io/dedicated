package userkey

import (
	"fmt"
	"strconv"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/nitrous-common/sshkey"
)

var (
	ErrNotFound              = fmt.Errorf("public key not found")
	ErrKeyBlank              = fmt.Errorf("missing public key")
	ErrInvalidKey            = fmt.Errorf("invalid public key")
	ErrDuplicateFingerprints = fmt.Errorf("multiple keys found for fingerprint")
)

type UserKey struct {
	ID          int64      `db:"id"`
	UserID      int64      `db:"user_id"`
	ContainerID int64      `db:"container_id"`
	Fingerprint string     `db:"fingerprint"`
	Key         string     `db:"key"`
	CreatedAt   *time.Time `db:"created_at"`
	UpdatedAt   *time.Time `db:"updated_at"`
}

func FindAll(db *db.Database, userID int64, containerID int64) ([]*UserKey, error) {
	keys := []*UserKey{}
	err := db.Select(&keys, "SELECT * FROM public_keys WHERE user_id = $1 AND container_id = $2 ORDER BY created_at", userID, containerID)
	if err != nil {
		return []*UserKey{}, err
	}
	return keys, nil
}

// Find queries for a Public Key scoped to a user and a container.
// identifier can be either an ID or a fingerprint.
func Find(db *db.Database, userID int64, containerID int64, identifier string) (*UserKey, error) {
	keys := []UserKey{}
	id, err := strconv.ParseInt(identifier, 10, 64)

	if err != nil {
		err = db.Select(&keys,
			"SELECT * FROM public_keys WHERE user_id = $1 AND container_id = $2 AND fingerprint = $3 ORDER BY created_at", userID, containerID, identifier)
	} else {
		err = db.Select(&keys,
			"SELECT * FROM public_keys WHERE user_id = $1 AND container_id = $2 AND id = $3 ORDER BY created_at", userID, containerID, id)
	}

	if len(keys) == 0 {
		return nil, ErrNotFound
	}
	if len(keys) > 1 {
		return nil, ErrDuplicateFingerprints
	}
	return &keys[0], err
}

func DestroyAll(db *db.Database, userID int64, containerID int64) error {
	_, err := db.Exec(`DELETE FROM public_keys WHERE user_id = $1 AND container_id = $2;`, userID, containerID)
	return err
}

func Destroy(db *db.Database, userID int64, containerID int64, id int64) error {
	_, err := db.Exec(`DELETE FROM public_keys WHERE user_id = $1 AND container_id = $2 AND id = $3;`, userID, containerID, id)
	return err
}

func (pk *UserKey) Insert(d db.QueryReceiver) error {
	// this is a very basic check for valid characters; it will not catch
	// keys like "ssh-rsa fail"
	fingerprint, err := pk.Md5Fingerprint()
	if err != nil {
		return ErrInvalidKey
	}
	pk.Fingerprint = fingerprint

	nstmt, err := d.PrepareNamed(`
		WITH selection AS (
			SELECT * FROM public_keys WHERE fingerprint = :fingerprint AND container_id = :container_id
		), insertion AS (
			INSERT INTO public_keys (user_id, container_id, key, fingerprint)
			(
				SELECT :user_id, :container_id, :key, :fingerprint
				WHERE NOT EXISTS (SELECT * FROM selection)
			)
			RETURNING *
		)
		(SELECT * FROM insertion)
		UNION
		(SELECT * FROM selection)
	`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pk).StructScan(pk)
}

// Fingerprint is set in Insert()
func (pk *UserKey) Validate() error {
	if pk.Key == "" {
		return ErrKeyBlank
	}

	// We can do more checks with
	// https://godoc.org/golang.org/x/crypto/ssh#ParsePublicKey but
	// seems overkill for now
	return nil
}

func (pk *UserKey) Md5Fingerprint() (string, error) {
	return sshkey.Md5Fingerprint(pk.Key)
}
