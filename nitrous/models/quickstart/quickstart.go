package quickstart

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Quickstart struct {
	ID          int64      `db:"id"`
	UserID      int64      `db:"user_id"`
	ContainerID int64      `db:"container_id"`
	TemplateID  int64      `db:"template_id"`
	RepoURL     string     `db:"repo_url"`
	CreatedAt   *time.Time `db:"created_at"`
	UpdatedAt   *time.Time `db:"updated_at"`
}

func (q *Quickstart) Insert(d db.QueryReceiver) error {
	s, err := d.PrepareNamed(`
		INSERT INTO quickstarts
		(user_id, container_id, template_id, repo_url)
		VALUES (:user_id, :container_id, :template_id, :repo_url)
		RETURNING *;
	`)
	if err != nil {
		return err
	}
	return s.QueryRowx(q).StructScan(q)
}
