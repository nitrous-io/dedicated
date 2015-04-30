package containercollab

import (
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type ContainerCollabUser struct {
	ContainerCollab

	Username string `db:"username"`
	Email    string `db:"email"`
	Name     string `db:"name"`
}

type ContainerCollab struct {
	ID          int64       `db:"id"`
	ContainerID int64       `db:"container_id"`
	UserID      int64       `db:"user_id"`
	EmailSentAt pq.NullTime `db:"email_sent_at"`
	CreatedAt   *time.Time  `db:"created_at"`
	UpdatedAt   *time.Time  `db:"updated_at"`
}

func Create(d db.QueryReceiver, cID, uID int64) (*ContainerCollab, error) {
	sc := &ContainerCollab{
		ContainerID: cID,
		UserID:      uID,
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO container_collabs (user_id, container_id) VALUES (:user_id, :container_id) RETURNING *;`)
	if err != nil {
		return nil, err
	}
	return sc, nstmt.QueryRowx(sc).StructScan(sc)
}

func Delete(d db.QueryReceiver, scID int64) error {
	_, err := d.Exec(`DELETE FROM container_collabs WHERE id = $1 RETURNING *;`, scID)
	return err
}

func FindAll(d db.QueryReceiver, cID int64) ([]*ContainerCollabUser, error) {
	shared := []*ContainerCollabUser{}
	err := d.Select(&shared, `SELECT
		container_collabs.*,
		users.username,
		users.email,
		users.name
		FROM container_collabs
		INNER JOIN users ON users.id = container_collabs.user_id
		WHERE container_id = $1 AND users.deleted_at IS NULL;`,
		cID)
	if err != nil {
		return nil, err
	}
	return shared, nil
}
