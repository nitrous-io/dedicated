package freequeue

import (
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

const (
	StatePending      = "pending"
	StateApproved     = "approved"
	StateRejected     = "rejected"
	StateProvisioning = "provisioning"
	StateProvisioned  = "provisioned"
	StateError        = "error"
)

var (
	ValidStates = []string{
		StatePending,
		StateApproved,
		StateRejected,
		StateProvisioning,
		StateProvisioned,
		StateError,
	}

	ErrInvalidState      = errors.New("Provided state is not valid")
	ErrInconsistentState = errors.New("Provided record was in an unexpected state")
)

type FreeQueue struct {
	ID     int64 `db:"id"`
	UserID int64 `db:"user_id"`

	State string `db:"state"`

	ActionedBy sql.NullInt64 `db:"actioned_by"`
	ActionedAt pq.NullTime   `db:"actioned_at"`

	ContainerTemplateSlug string `db:"container_template_slug"`
	ContainerName         string `db:"container_name"`
	HostName              string `db:"host_name"`
	RegionSlug            string `db:"region_slug"`

	CreatedAt *time.Time  `db:"created_at"`
	UpdatedAt *time.Time  `db:"updated_at"`
	DeletedAt pq.NullTime `db:"deleted_at"`
}

type FreeQueueWithUser struct {
	FreeQueue

	Username         string         `db:"username"`
	Name             string         `db:"name"`
	Email            string         `db:"email"`
	Phone            sql.NullString `db:"phone"`
	PhoneCountryCode sql.NullString `db:"phone_country_code"`
	PhoneAuthed      bool           `db:"phone_authed"`
	EmailConfirmed   bool           `db:"email_confirmed"`
}

func (f *FreeQueue) Insert(db *db.Database) error {
	nstmt, err := db.PrepareNamed(`INSERT INTO free_queue
    (user_id, container_template_slug, container_name, host_name, region_slug)
    VALUES (:user_id, :container_template_slug, :container_name, :host_name, :region_slug)
    RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRow(f).StructScan(f)
}

func Action(tx *sqlx.Tx, id int64, expectedState, newState string, userID int64) (*FreeQueue, error) {
	validState := false
	for _, st := range ValidStates {
		if st == newState {
			validState = true
			break
		}
	}

	if !validState {
		return nil, ErrInvalidState
	}

	var success bool
	err := tx.QueryRow("SELECT true FROM free_queue WHERE id = $1 AND deleted_at IS NULL AND actioned_by IS NULL AND state = $2 FOR UPDATE;", id, expectedState).Scan(&success)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInconsistentState
		}
		return nil, err
	}
	if !success {
		return nil, ErrInconsistentState
	}

	fq := &FreeQueue{}
	err = tx.QueryRowx(`UPDATE free_queue
		SET state = $1, actioned_by = $2, actioned_at = now(), updated_at = now()
		WHERE id = $3 RETURNING *;`, newState, userID, id).StructScan(fq)
	if err != nil {
		return nil, err
	}

	return fq, nil
}

func Cancel(db *db.Database, userID int64) error {
	_, err := db.Exec(`UPDATE free_queue SET deleted_at = now() WHERE user_id = $1 AND deleted_at IS NULL;`, userID)
	return err
}

func Find(db *db.Database, userID int64) (*FreeQueue, error) {
	fq := &FreeQueue{}
	if err := db.Get(fq, `SELECT * FROM free_queue WHERE user_id = $1 AND deleted_at IS NULL;`, userID); err != nil {
		return nil, err
	}
	return fq, nil
}

func FindAllPending(db *db.Database) ([]*FreeQueueWithUser, error) {
	return findWithState(db, StatePending)
}

func FindAllApproved(db *db.Database) ([]*FreeQueueWithUser, error) {
	return findWithState(db, StateApproved)
}

func findWithState(db *db.Database, state string) ([]*FreeQueueWithUser, error) {
	fq := []*FreeQueueWithUser{}
	err := db.Select(&fq, `SELECT
		free_queue.*,
		users.username, users.name, users.email, users.phone, users.phone_country_code, users.phone_authed,
		(users.confirmed_at IS NOT NULL) email_confirmed
		FROM free_queue
		INNER JOIN users ON free_queue.user_id = users.id
		WHERE state = $1
		AND free_queue.deleted_at IS NULL
		AND users.deleted_at IS NULL
		ORDER BY free_queue.created_at ASC;`, state)
	if err != nil {
		return nil, err
	}
	return fq, nil
}
