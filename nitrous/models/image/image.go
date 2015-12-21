package image

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/parsers"
)

type Image struct {
	ID     int64 `db:"id"`
	UserID int64 `db:"user_id"`
	HostID int64 `db:"host_id"`

	FreighterID string `db:"freighter_id"`
	// Name is for display purposes only.
	Name string `db:"name"`
	// Slug is actually the equivalent of a Docker image repository without
	// the optional tag, e.g. "ubuntu" or "quay.io/nitrousio/scratch"
	Slug string `db:"slug"`
	Tag  string `db:"tag"`

	// OriginalImage is the original Docker image that the container was created
	// from. Applies only to snapshots.
	OriginalImage sql.NullString `db:"original_image"`
	IsSnapshot    bool           `db:"is_snapshot"`

	DeletedAt pq.NullTime `db:"deleted_at"`
	CreatedAt *time.Time  `db:"created_at"`
	UpdatedAt *time.Time  `db:"updated_at"`

	// These fields come from Freighter.
	ActualSize  int64
	VirtualSize int64
	// BuiltAt is time that Docker reports the image was created.
	BuiltAt *time.Time
}

var DefaultTag = "latest"

func FindAll(d db.QueryReceiver, u *user.User, h *host.Host) ([]*Image, error) {
	var err error
	images := []*Image{}
	if h == nil {
		err = d.Select(&images, `SELECT * FROM images WHERE user_id = $1 AND deleted_at IS NULL ORDER BY id;`, u.ID)
	} else {
		err = d.Select(&images, `SELECT * FROM images WHERE user_id = $1 AND host_id = $2 AND deleted_at IS NULL ORDER BY id;`, u.ID, h.ID)
	}
	if err != nil {
		return nil, err
	}
	return images, nil
}

// Find returns the image on the given host with the given identifier. An
// identifier can be either a FID (Freighter ID) or slug (with optional tag,
// i.e. slug[:tag]).
func Find(d db.QueryReceiver, u *user.User, h *host.Host, identifier string) (*Image, error) {
	var (
		img   Image
		nstmt *sqlx.NamedStmt
		err   error
	)
	repo, tag := ParseRepoAndTag(identifier)
	if tag == "" {
		tag = DefaultTag
	}
	params := map[string]interface{}{
		"user_id": u.ID,
		"host_id": h.ID,
		"slug":    repo,
		"tag":     tag,
	}
	// if parseInt returns an error it means
	// given identifier is not an id.
	id, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		nstmt, err = d.PrepareNamed(`SELECT * FROM images
			WHERE user_id = :user_id AND host_id = :host_id
			AND slug = :slug AND tag = :tag
			AND deleted_at IS NULL`)
	} else {
		params["id"] = id
		nstmt, err = d.PrepareNamed(`SELECT * FROM images
			WHERE user_id = :user_id AND host_id = :host_id
			AND id = :id
			AND deleted_at IS NULL`)
	}
	if err != nil {
		return nil, err
	}
	if err := nstmt.Get(&img, params); err != nil {
		return nil, err
	}
	return &img, nil
}

func (i *Image) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO images
		(user_id, host_id, freighter_id, name, slug, tag, original_image, is_snapshot)
    VALUES (:user_id, :host_id, :freighter_id, :name, :slug, :tag, :original_image, :is_snapshot)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}

func (i *Image) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE images
		SET (user_id, host_id, freighter_id, name, slug, tag, original_image, is_snapshot, updated_at)
		= (:user_id, :host_id, :freighter_id, :name, :slug, :tag, :original_image, :is_snapshot, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}

func (i *Image) InsertOrUpdate(d db.QueryReceiver) error {
	if i.ID == 0 {
		return i.Insert(d)
	}
	return i.Update(d)
}

func (i *Image) Delete(d db.QueryReceiver) error {
	_, err := d.Exec(`UPDATE images SET deleted_at = now() WHERE id = $1`, i.ID)
	return err
}

// FullRepository returns the full repository of the image (i.e. including its
// tag). If for some reason tag is empty, DefaultTag is assumed.
func (i *Image) FullRepository() string {
	tag := i.Tag
	if tag == "" {
		tag = DefaultTag
	}
	return fmt.Sprintf("%s:%s", i.Slug, tag)
}

func (i *Image) Stack() string {
	if !i.OriginalImage.Valid {
		return ""
	}
	return parsers.ParseStack(i.OriginalImage.String)
}

// ParseRepoAndTag extracts the repo name and tag from a repository string.
// Examples:
//    ubuntu -> "ubuntu" + ""
//    ubuntu:14.04 -> "ubuntu" + "14.04"
//    nitrousio/ubuntu:14.04 -> "nitrousio/ubuntu" + "14.04"
//    example.com:5000/ubuntu:14.04 -> "example.com:5000/ubuntu" + "14.04"
//    example.com:5000/nitrousio/ubuntu:14.04 -> "example.com:5000/nitrousio/ubuntu" + "14.04"
// Stolen from https://github.com/docker/docker/blob/403df1765abfd3ac62b7f6b8bd709fb0d691d8c8/pkg/parsers/parsers.go#L72.
func ParseRepoAndTag(repos string) (string, string) {
	n := strings.LastIndex(repos, ":")
	if n < 0 {
		return repos, ""
	}
	tag := repos[n+1:]
	if !strings.Contains(tag, "/") {
		return repos[:n], tag
	}
	return repos, ""
}
