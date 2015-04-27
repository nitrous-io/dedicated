// model for container<->volume many-to-many
package cvolume

import (
	"errors"
	"time"

	"github.com/lib/pq"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
)

type ContainerVolume struct {
	ID          int64      `db:"id"`
	ContainerID int64      `db:"container_id"`
	VolumeID    int64      `db:"volume_id"`
	CreatedAt   *time.Time `db:"created_at"`
}

var (
	ErrAlreadyAssociated = errors.New("container and volume are already associated")
)

func Find(d db.QueryReceiver, c *container.Container, v *volume.Volume) (*ContainerVolume, error) {
	cv := &ContainerVolume{}
	err := d.Get(cv, `SELECT cv.id, container_id, volume_id, cv.created_at
		FROM container_volumes cv
		JOIN containers ON containers.id = container_id
		WHERE container_id = $1 AND volume_id = $2
		  AND terminated_at IS NULL
		`, c.ID, v.ID)
	if err != nil {
		return nil, err
	}
	return cv, nil
}

// FindAll returns all volumes associated with a container
func FindAll(d db.QueryReceiver, c *container.Container) ([]*volume.Volume, error) {
	vols := []*volume.Volume{}
	err := d.Select(&vols, `SELECT v.*
	FROM volumes v JOIN container_volumes cv ON v.id = cv.volume_id
	WHERE cv.container_id = $1`, c.ID)
	if err != nil {
		return nil, err
	}
	return vols, nil
}

func DeleteContainer(d db.QueryReceiver, c *container.Container) error {
	_, err := d.Exec(`DELETE FROM container_volumes WHERE container_id = $1`, c.ID)
	return err
}

func (cv *ContainerVolume) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO container_volumes
		(container_id, volume_id) VALUES
		(:container_id, :volume_id)
		RETURNING *;`)
	if err != nil {
		return err
	}
	err = nstmt.QueryRowx(cv).StructScan(cv)
	if err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if e.Constraint == "index_container_volumes_on_container_id_and_volume_id" {
				return ErrAlreadyAssociated
			}
		}
	}
	return err
}

func (cv *ContainerVolume) Delete(d db.QueryReceiver) error {
	_, err := d.Exec(`DELETE FROM container_volumes WHERE id = $1`, cv.ID)
	return err
}

func IsContainerExist(d db.QueryReceiver, v *volume.Volume) (bool, error) {
	var rows []int
	err := d.Select(&rows, `SELECT 1 FROM container_volumes WHERE volume_id = $1 LIMIT 1`, v.ID)
	if err != nil {
		return false, err
	}

	if len(rows) > 0 {
		return true, nil
	}
	return false, err
}
