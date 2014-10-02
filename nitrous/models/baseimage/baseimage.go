package baseimage

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type BaseImage struct {
	ID          int64      `db:"id"`
	Name        string     `db:"name"`
	Repo        string     `db:"repo"`
	Description string     `db:"description"`
	CreatedAt   *time.Time `db:"created_at"`
	UpdatedAt   *time.Time `db:"updated_at"`
}

func All(d db.QueryReceiver) ([]*BaseImage, error) {
	imgs := []*BaseImage{}
	if err := d.Select(&imgs, `SELECT * FROM base_images`); err != nil {
		return nil, err
	}
	return imgs, nil
}

func (b *BaseImage) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO base_images (name, repo, description) VALUES (:name, :repo, :description) RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(b).StructScan(b)
}
