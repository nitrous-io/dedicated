package image

import (
	"fmt"
	"strings"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/models/user"
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

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`

	// These fields come from Freighter.
	ActualSize  int64
	VirtualSize int64
}

var DefaultTag = "latest"

// Find returns the image on the given host with the given identifier. An
// identifier can be either a FID (Freighter ID) or slug (with optional tag,
// i.e. slug[:tag]).
func Find(d db.QueryReceiver, u *user.User, h *host.Host, identifier string) (*Image, error) {
	var i Image
	repo, tag := ParseRepoAndTag(identifier)
	if tag == "" {
		tag = DefaultTag
	}
	err := d.Get(&i, `SELECT * FROM images
		WHERE user_id = $1 AND host_id = $2 AND
		(freighter_id = $3 OR (slug = $4 AND tag = $5))`,
		u.ID, h.ID,
		identifier, repo, tag)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (i *Image) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO images
		(user_id, host_id, freighter_id, name, slug, tag, created_at)
    VALUES (:user_id, :host_id, :freighter_id, :name, :slug, :tag, :created_at)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(i).StructScan(i)
}

func (i *Image) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE images
		SET (user_id, host_id, freighter_id, name, slug, tag, created_at)
		= (:user_id, :host_id, :freighter_id, :name, :slug, :tag, :created_at)
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
	_, err := d.Exec(`DELETE FROM images WHERE id = $1`, i.ID)
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
