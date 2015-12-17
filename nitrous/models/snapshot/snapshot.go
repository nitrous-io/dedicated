package snapshot

import (
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/pkg/parsers"
)

type Snapshot struct {
	ID      int64         `db:"id"`
	ImageID int64         `db:"image_id"`
	UserID  int64         `db:"user_id"`
	OrgID   sql.NullInt64 `db:"org_id"`

	Slug          string        `db:"slug"`
	Namespace     string        `db:"namespace"`
	Name          string        `db:"name"`
	Tag           string        `db:"tag"`
	DisplayName   string        `db:"display_name"`
	OriginalImage string        `db:"original_image"`
	Size          sql.NullInt64 `db:"size"`

	Downloads int64 `db:"downloads"`

	CreatedAt pq.NullTime `db:"created_at"`
	UpdatedAt pq.NullTime `db:"updated_at"`
}

type SnapshotWithOwner struct {
	*Snapshot

	OwnerUsername sql.NullString `db:"owner_username"`
	OwnerName     sql.NullString `db:"owner_name"`
	OwnerEmail    sql.NullString `db:"owner_email"`
}

func FindAllByOrgID(d db.QueryReceiver, userID, orgID int64) ([]*SnapshotWithOwner, error) {
	snapshots := []*SnapshotWithOwner{}
	err := d.Select(&snapshots, `
		SELECT snapshots.*,
			users.username AS owner_username,
			users.name AS owner_name,
			users.email AS owner_email
		FROM snapshots
			LEFT JOIN users ON users.id = snapshots.user_id
			JOIN orgs ON snapshots.org_id = orgs.id
			JOIN org_memberships ON orgs.id = org_memberships.org_id
		WHERE org_memberships.user_id = $1 AND snapshots.org_id = $2;`,
		userID, orgID)
	if err != nil {
		return nil, err
	}
	return snapshots, nil
}

func FindByID(d db.QueryReceiver, id int64) (*SnapshotWithOwner, error) {
	s := &SnapshotWithOwner{}
	if err := d.Get(s, `SELECT snapshots.*,
			users.username AS owner_username,
			users.name AS owner_name,
			users.email AS owner_email
		FROM snapshots
			LEFT JOIN users ON users.id = snapshots.user_id
		WHERE snapshots.id = $1;`, id); err != nil {
		return nil, err
	}
	return s, nil
}

func FindByImageID(d db.QueryReceiver, imageID int64) (*SnapshotWithOwner, error) {
	s := &SnapshotWithOwner{}
	if err := d.Get(s, `SELECT snapshots.*,
			users.username AS owner_username,
			users.name AS owner_name,
			users.email AS owner_email
		FROM snapshots
			LEFT JOIN users ON users.id = snapshots.user_id
		WHERE snapshots.image_id = $1;`, imageID); err != nil {
		return nil, err
	}
	return s, nil
}

func FindBySlug(d db.QueryReceiver, slug string) (*SnapshotWithOwner, error) {
	s := &SnapshotWithOwner{}
	if err := d.Get(s, `SELECT snapshots.*,
			users.username AS owner_username,
			users.name AS owner_name,
			users.email AS owner_email
		FROM snapshots
			LEFT JOIN users ON users.id = snapshots.user_id
		WHERE snapshots.slug = $1;`, slug); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Snapshot) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO snapshots
		(image_id, user_id, org_id, slug, namespace, name, tag, display_name, original_image, size, downloads)
		VALUES (:image_id, :user_id, :org_id, :slug, :namespace, :name, :tag, :display_name, :original_image, :size, :downloads)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(s).StructScan(s)
}

func (s *Snapshot) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE snapshots
		SET (image_id, user_id, org_id, slug, namespace, name, tag, display_name, original_image, size, downloads, updated_at)
		= (:image_id, :user_id, :org_id, :slug, :namespace, :name, :tag, :display_name, :original_image, :size, :downloads, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(s).StructScan(s)
}

func (s *Snapshot) IncrementDownloads(d db.QueryReceiver) error {
	_, err := d.Exec(`UPDATE snapshots SET downloads = downloads + 1 WHERE id = $1;`, s.ID)
	return err
}

func (s *Snapshot) Delete(d db.QueryReceiver) error {
	_, err := d.Exec(`DELETE FROM snapshots WHERE id = $1;`, s.ID)
	return err
}

func (s *Snapshot) Stack() string {
	return parsers.ParseStack(s.OriginalImage)
}

// Repository returns the fully specified Docker image repository.
func (s *Snapshot) Repository() string {
	return fmt.Sprintf("%s/%s:%s", s.Namespace, s.Name, s.Tag)
}
