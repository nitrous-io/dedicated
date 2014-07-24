package publickey

import (
	"fmt"
	"time"

	"github.com/nitrous-io/nitrous-common/sshkey"
	"github.com/nitrous-io/nitrous/db"
)

var (
	ErrNotFound              = fmt.Errorf("public key not found")
	ErrKeyBlank              = fmt.Errorf("missing public key")
	ErrInvalidKey            = fmt.Errorf("invalid public key")
	ErrDuplicateFingerprints = fmt.Errorf("multiple keys found for fingerprint")
)

type PublicKey struct {
	ID          int64      `db:"id" json:"id"`
	UserID      int64      `db:"user_id" json:"-"`
	ContainerID int64      `db:"container_id" json:"container_id"`
	Fingerprint string     `db:"fingerprint" json:"fingerprint"`
	Key         string     `db:"key" json:"key"`
	CreatedAt   *time.Time `db:"created_at" json:"-"`
	UpdatedAt   *time.Time `db:"updated_at" json:"-"`
}

func FindAllByUserIDAndContainerID(db *db.Database, userID int64, containerID int64) ([]PublicKey, error) {
	keys := []PublicKey{}
	err := db.Select(&keys, "SELECT * FROM public_keys WHERE user_id = $1 AND container_id = $2 ORDER BY created_at", userID, containerID)
	if err != nil {
		return []PublicKey{}, err
	}
	return keys, nil
}

func FindByUserIDAndContainerIDAndFingerprint(db *db.Database, userID int64, containerID int64, fprint string) (*PublicKey, error) {
	keys := []PublicKey{}
	err := db.Select(&keys, "SELECT * FROM public_keys WHERE user_id = $1 AND container_id = $2 AND fingerprint = $3 ORDER BY created_at", userID, containerID, fprint)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, ErrNotFound
	}
	if len(keys) > 1 {
		return nil, ErrDuplicateFingerprints
	}
	return &keys[0], err
}

func DestroyAllByUserIDAndContainerID(db *db.Database, userID int64, containerID int64) error {
	_, err := db.Queryx("DELETE FROM public_keys WHERE user_id = $1 AND container_id = $2", userID, containerID)
	return err
}

func DestroyByUserIDAndContainerIDAndFingerprint(db *db.Database, userID int64, containerID int64, fingerprint string) error {
	_, err := db.Exec(`DELETE FROM public_keys
		WHERE user_id = $1 AND container_id = $2 AND fingerprint = $3;`,
		userID, containerID, fingerprint)
	if err != nil {
		return err
	}
	return nil
}

func (pk *PublicKey) Insert(d db.QueryReceiver) error {
	// this is a very basic check for valid characters; it will not catch
	// keys like "ssh-rsa fail"
	fingerprint, err := pk.Md5Fingerprint()
	if err != nil {
		return ErrInvalidKey
	}
	pk.Fingerprint = fingerprint

	nstmt, err := d.PrepareNamed(`INSERT INTO public_keys
		(user_id, container_id, key, fingerprint)
		VALUES (:user_id, :container_id, :key, :fingerprint)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pk).StructScan(pk)
}

// Fingerprint is set in Insert()
func (pk *PublicKey) Validate() error {
	if pk.Key == "" {
		return ErrKeyBlank
	}
	// We can do more checks with
	// https://godoc.org/code.google.com/p/go.crypto/ssh#ParsePublicKey but
	// seems overkill for now
	return nil
}

func (pk *PublicKey) Md5Fingerprint() (string, error) {
	return sshkey.Md5Fingerprint(pk.Key)
}
