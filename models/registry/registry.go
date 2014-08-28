package registry

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/nitrous-io/nitrous/db"
)

type Registry struct {
	ID        int64      `db:"id"`
	UserID    int64      `db:"user_id"`
	HostID    int64      `db:"host_id"`
	URL       string     `db:"url"`
	Username  string     `db:"username"`
	Email     string     `db:"email"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func FindAll(db *db.Database, userID int64, hostID int64) ([]*Registry, error) {
	registries := []*Registry{}
	err := db.Select(&registries, "SELECT * FROM registries WHERE user_id = $1 AND host_id = $2 ORDER BY created_at", userID, hostID)
	if err != nil {
		return []*Registry{}, err
	}
	return registries, nil
}

// Find queries for a registry entry scoped by the user and host.
// identifier can be either an ID or a registry URL.
func Find(d db.QueryReceiver, userID int64, hostID int64, identifier string) (*Registry, error) {
	r := &Registry{}

	id, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		err = d.Get(r,
			`SELECT * FROM registries WHERE user_id = $1 AND host_id = $2 AND url = $3;`, userID, hostID, identifier)
	} else {
		err = d.Get(r,
			`SELECT * FROM registries WHERE user_id = $1 AND host_id = $2 AND id = $3;`, userID, hostID, id)
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return r, nil
}

func (r *Registry) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO registries
		(user_id, host_id, url, username, email)
		VALUES (:user_id, :host_id, :url, :username, :email)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(r).StructScan(r)
}

func Delete(db *db.Database, userID int64, ID int64) error {
	_, err := db.Exec("DELETE FROM registries WHERE user_id = $1 AND id = $2", userID, ID)
	return err
}
