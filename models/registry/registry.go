package registry

import (
	"time"

	"github.com/nitrous-io/nitrous/db"
)

type Registry struct {
	ID          int64      `db:"id" json:"id"`
	UserID      int64      `db:"user_id" json:"-"`
	HostID      int64      `db:"host_id" json:"host_id"`
	RegistryURL string     `db:"registry_url" json:"registry_url"`
	Username    string     `db:"username" json:"username"`
	Email       string     `db:"email" json:"email"`
	CreatedAt   *time.Time `db:"created_at" json:"-"`
	UpdatedAt   *time.Time `db:"updated_at" json:"-"`
}

func FindAllByUserIDAndHostID(db *db.Database, userID int64, hostID int64) ([]Registry, error) {
	registries := []Registry{}
	err := db.Select(&registries, "SELECT * FROM registries WHERE user_id = $1 AND host_id = $2 ORDER BY created_at", userID, hostID)
	if err != nil {
		return []Registry{}, err
	}
	return registries, nil
}

func (r *Registry) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO registries
		(user_id, host_id, registry_url, username, email)
		VALUES (:user_id, :host_id, :registry_url, :username, :email)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(r).StructScan(r)
}
