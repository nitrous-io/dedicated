package customdomain

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type CustomDomain struct {
	ID            int64      `db:"id"`
	ContainerID   int64      `db:"container_id"`
	Domain        string     `db:"domain"`
	Port          int64      `db:"port"`
	ContainerPort int64      `db:"container_port"`
	CreatedAt     *time.Time `db:"created_at"`
	UpdatedAt     *time.Time `db:"updated_at"`
}

func (cd *CustomDomain) Insert(d db.QueryReceiver) error {
	s, err := d.PrepareNamed(`
		INSERT INTO custom_domains
		(container_id, domain, port, container_port)
		VALUES (:container_id, :domain, :port, :container_port)
		RETURNING *;
	`)
	if err != nil {
		return err
	}
	return s.QueryRowx(cd).StructScan(cd)
}
