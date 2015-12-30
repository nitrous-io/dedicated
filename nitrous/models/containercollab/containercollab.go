package containercollab

import (
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

var (
	ErrCollabLimitReached = errors.New("collaborators limit reached")
	ErrDoesNotExist       = errors.New("org membership does not exist")
)

type ContainerCollabUser struct {
	ContainerCollab

	Username sql.NullString `db:"username"`
	Email    sql.NullString `db:"email"`
	Name     sql.NullString `db:"name"`
}

type ContainerCollab struct {
	ID          int64         `db:"id"`
	ContainerID int64         `db:"container_id"`
	UserID      sql.NullInt64 `db:"user_id"`
	EmailSentAt pq.NullTime   `db:"email_sent_at"`

	InviteEmail sql.NullString `db:"invite_email"`
	InviteCode  sql.NullString `db:"invite_code"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func Create(d db.QueryReceiver, cID, uID int64) (*ContainerCollab, error) {
	sc := &ContainerCollab{
		ContainerID: cID,
		UserID:      sql.NullInt64{uID, true},
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO container_collabs (user_id, container_id) VALUES (:user_id, :container_id) RETURNING *;`)
	if err != nil {
		return nil, err
	}
	return sc, nstmt.QueryRowx(sc).StructScan(sc)
}

func Delete(d db.QueryReceiver, scID, cID int64) error {
	_, err := d.Exec(`DELETE FROM container_collabs WHERE id = $1 AND container_id = $2 RETURNING *;`, scID, cID)
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
		LEFT JOIN users ON users.id = container_collabs.user_id
		WHERE container_id = $1 AND users.deleted_at IS NULL
		ORDER BY COALESCE(users.username, invite_email);`,
		cID)
	if err != nil {
		return nil, err
	}
	return shared, nil
}

func GetCollaboratorIDs(d db.QueryReceiver, cID int64) ([]int64, error) {
	var userIDs []int64
	err := d.Select(&userIDs, `SELECT user_id FROM container_collabs WHERE container_id = $1;`, cID)
	if err != nil {
		return userIDs, err
	}
	return userIDs, nil
}

// max_collaborators on plans has a default value of 1. Where org=true, max_collaborators is set to NULL.
func CanAddCollaborator(d db.QueryReceiver, cnID int64) error {
	var can bool
	if err := d.QueryRowx(`SELECT
	  (COUNT(cc.id) < COALESCE(MIN(p.max_collaborators), 0) OR MIN(p.max_collaborators) IS NULL)
		FROM containers c
		JOIN container_collabs cc ON (cc.container_id = c.id)
		JOIN hosts h ON (h.id = c.host_id)
		JOIN plans p ON (p.id = h.plan_id) WHERE c.id=$1;`,
		cnID).Scan(&can); err != nil {
		return err
	}
	if !can {
		return ErrCollabLimitReached
	}
	return nil
}

func PromotePendingMemberToUser(d db.QueryReceiver, email, invitecode string, userID int64) (*ContainerCollab, error) {
	var exists bool
	err := d.Get(&exists,
		`SELECT COUNT(*) = 1
		FROM container_collabs
		WHERE invite_email = $1
		AND invite_code = $2
		LIMIT 1`, email, invitecode)
	if err != nil {
		return nil, err
	}

	if !exists {
		return nil, ErrDoesNotExist
	}

	cc := &ContainerCollab{
		UserID:      sql.NullInt64{userID, true},
		InviteEmail: sql.NullString{email, true},
	}
	nstmt, err := d.PrepareNamed(
		`UPDATE container_collabs
		SET (user_id, invite_email, invite_code)
		= (:user_id, NULL, NULL)
		WHERE invite_email = :invite_email 
		RETURNING *;`)
	err = nstmt.QueryRowx(cc).StructScan(cc)
	return cc, err
}
