package announcement

import (
	"time"

	dbLib "github.com/nitrous-io/dedicated/nitrous/db"
)

type HostOwner struct {
	HostID   int64  `db:"host_id"`
	UserID   int64  `db:"user_id"`
	Username string `db"username"`
	Fullname string `db:"fullname"`
	Email    string `db:"email"`
	Slug     string `db:"slug"`
}

type Announcement struct {
	ID        int64      `db:"id"`
	Subject   string     `db:"subject"`
	Message   string     `db:"message"`
	ShowToAll bool       `db:"show_to_all"`
	ExpiresAt *time.Time `db:"expires_at"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

type HostAnnouncement struct {
	Announcement
	HostOwner
}

func FindHostOwnerByHostID(db *dbLib.Database, hostID int64) (*HostOwner, error) {
	owner := &HostOwner{}

	err := db.Select(owner, `SELECT
    hosts.id AS host_id, users.id AS user_id, username, users.name as fullname, email 
    FROM hosts INNER JOIN users ON (hosts.user_id = users.id)
    WHERE hosts.terminated_at IS NULL AND hosts.id=$1;
  `, hostID)

	if err != nil {
		return nil, err
	}

	return owner, nil
}

func FindAllHostOwners(db *dbLib.Database) ([]*HostOwner, error) {
	owners := []*HostOwner{}

	err := db.Select(&owners, `SELECT
    hosts.id AS host_id, users.id AS user_id, username, users.name as fullname, email 
    FROM hosts INNER JOIN users ON (hosts.user_id = users.id)
    WHERE hosts.terminated_at IS NULL;
  `)

	if err != nil {
		return nil, err
	}

	return owners, nil
}

func FindForHost(db *dbLib.Database, hostID int64) ([]*HostAnnouncement, error) {
	announcements := []*HostAnnouncement{}

	err := db.Select(&announcements, `SELECT 
    announcements.*, hosts.id AS host_id, users.id AS user_id, username, users.name as fullname, email 
    FROM announcements 
      INNER JOIN hosts ON (announcements.host_id = hosts.id)
      INNER JOIN users ON (hosts.user_id = users.id)
    WHERE hosts.terminated_at IS NULL AND hosts.id=$1;`, hostID)

	if err != nil {
		return nil, err
	}

	return announcements, nil
}

func FindForGlobal(db *dbLib.Database) ([]*Announcement, error) {
	announcements := []*Announcement{}

	err := db.Select(&announcements, `SELECT 
		id, subject, message, show_to_all, expires_at, created_at, updated_at
		FROM announcements WHERE show_to_all=true;`)

	if err != nil {
		return nil, err
	}

	return announcements, nil
}

func InsertForGlobal(db *dbLib.Database, announcement *Announcement) error {
	nstmt, err := db.PrepareNamed(`INSERT INTO announcement
    (subject, message, host_id, show_to_all, expires_at, created_at, updated_at)
    VALUES
    (:subject, :message, NULL, true, :expires_at, now(), now())
    RETURNING *;`)

	if err != nil {
		return err
	}
	return nstmt.QueryRowx(announcement).StructScan(announcement)
}

func InsertForHost(db *dbLib.Database, announcement *HostAnnouncement) error {
	nstmt, err := db.PrepareNamed(`INSERT INTO announcement
    (subject, message, host_id, show_to_all, expires_at, created_at, updated_at)
    VALUES
    (:subject, :message, :host_id, false, :expires_at, now(), now())
    RETURNING *;`)

	if err != nil {
		return err
	}
	return nstmt.QueryRowx(announcement).StructScan(announcement)
}

func CreateForHost(db *dbLib.Database, hostID int64, subject string, message string) (*HostAnnouncement, error) {
	owner, err := FindHostOwnerByHostID(db, hostID)
	if err != nil {
		return nil, err
	}

	announcement := &HostAnnouncement{
		Announcement: Announcement{
			Subject:   subject,
			Message:   message,
			ShowToAll: false,
		},
		HostOwner: *owner,
	}

	return announcement, nil
}
