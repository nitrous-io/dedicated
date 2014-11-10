package publickey

import (
	"database/sql"
	"fmt"
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

type PublicKey struct {
	ID          int64         `db:"id"`
	UserID      int64         `db:"user_id"`
	ContainerID sql.NullInt64 `db:"container_id"`
	HostID      sql.NullInt64 `db:"host_id"`
	Fingerprint string        `db:"fingerprint"`
	Key         string        `db:"key"`
	CreatedAt   *time.Time    `db:"created_at"`
	UpdatedAt   *time.Time    `db:"updated_at"`
}

type Inserter interface {
	Insert(d db.QueryReceiver) error
	Validate() error
}

// RawInsert is not intended to be called directly apart from ckey.Insert and
// hkey.Insert
func (pk *PublicKey) RawInsert(d db.QueryReceiver) error {
	// this is a very basic check for valid characters; it will not catch
	// keys like "ssh-rsa fail"
	fingerprint, err := sshkey.Md5Fingerprint(pk.Key)
	if err != nil {
		return ErrInvalidKey
	}
	pk.Fingerprint = fingerprint

	nstmt, err := d.PrepareNamed(`INSERT INTO public_keys
		(user_id, container_id, host_id, key, fingerprint)
		VALUES (:user_id, :container_id, :host_id, :key, :fingerprint)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pk).StructScan(pk)
}

// Fingerprint is set in RawInsert()
func (pk *PublicKey) Validate() error {
	if pk.Key == "" {
		return ErrKeyBlank
	}
	// We can do more checks with
	// https://godoc.org/code.google.com/p/go.crypto/ssh#ParsePublicKey but
	// seems overkill for now
	return nil
}
