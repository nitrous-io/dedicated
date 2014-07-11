package container

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/nitrous/db"
)

// FIXME add accompanying tables to lookup which users have access to a
// container (e.g. collaborators, users in the same team/organization).
type Container struct {
	ID        string         `db:"id"         json:"id"`
	UserID    int64          `db:"user_id"    json:"user_id"`
	HostID    int64          `db:"host_id"    json:"host_id"`
	Image     string         `db:"image"      json:"image"`
	Name      string         `db:"name"       json:"name"`
	LongName  sql.NullString `db:"long_name"  json:"long_name"`
	IPAddress sql.NullString `db:"ip_address" json:"ip_address"`

	TerminatedAt *time.Time `db:"terminated_at" json:"terminated_at"`
	TouchedAt    *time.Time `db:"touched_at"    json:"touched_at"`
	CreatedAt    *time.Time `db:"created_at"    json:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"    json:"updated_at"`

	State string `json:"state"`
}

func FindByID(d db.QueryReceiver, id string) (*Container, error) {
	c := &Container{}
	err := d.Get(c, `SELECT * FROM containers WHERE id = $1;`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func FindByHostIDAndName(d db.QueryReceiver, hostID int64, name string) (*Container, error) {
	c := &Container{}
	err := d.Get(c, `SELECT * FROM containers WHERE host_id = $1 AND name = $2;`, hostID, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func FindAllByHostID(d db.QueryReceiver, hostID int64) ([]*Container, error) {
	containers := []*Container{}
	err := d.Select(&containers, `SELECT * FROM containers WHERE host_id = $1;`, hostID)
	if err != nil {
		return []*Container{}, err
	}
	return containers, nil
}

func (c *Container) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO containers
		(id, user_id, host_id, image, name, long_name, ip_address)
		VALUES (:id, :user_id, :host_id, :image, :name, :long_name, :ip_address)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}

func (c *Container) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE containers
		SET (name, long_name, ip_address, terminated_at, touched_at, updated_at)
		= (:name, :long_name, :ip_address, :terminated_at, :touched_at, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}
