package volume

import (
	"database/sql"
	"errors"
	"regexp"
	"time"

	"github.com/nitrous-io/nitrous/db"
)

type Volume struct {
	ID        int64      `db:"id"`
	HostID    int64      `db:"host_id"`
	Name      string     `db:"name"`
	Slug      string     `db:"slug"`
	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

var slugRe = regexp.MustCompile(`\A[A-Za-z0-9][A-Za-z0-9_\-]*\z`)

var (
	ErrSlugBlank    = errors.New("slug can't be blank")
	ErrSlugInvalid  = errors.New("slug is not valid")
	ErrSlugTooShort = errors.New("slug is too short (minimum is 2 characters)")
	ErrSlugTooLong  = errors.New("slug is too long (maximum is 127 characters)")
)

func FindByHostIDAndSlug(d db.QueryReceiver, hid int64, slug string) (*Volume, error) {
	v := &Volume{}
	err := d.Get(v, `SELECT * FROM volumes WHERE host_id = $1 AND slug = $2`, hid, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return v, nil
}

func FindAllByHostID(d db.QueryReceiver, hid int64) ([]*Volume, error) {
	vols := []*Volume{}
	err := d.Select(&vols, `SELECT * FROM volumes WHERE host_id = $1 ORDER BY slug`, hid)
	if err != nil {
		return nil, err
	}
	return vols, nil
}

func (v *Volume) Insert(d db.QueryReceiver) error {
	if v.Name == "" {
		v.Name = v.Slug
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO volumes
		(host_id, slug, name) VALUES
		(:host_id, :slug, :name)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(v).StructScan(v)
}

func (v *Volume) Delete(d db.QueryReceiver) error {
	if _, err := d.Exec(`DELETE FROM container_volumes WHERE volume_id = $1`, v.ID); err != nil {
		return err
	}
	_, err := d.Exec(`DELETE FROM volumes WHERE id = $1`, v.ID)
	return err
}

func (v *Volume) Validate() error {
	slugLen := len(v.Slug)
	switch {
	case v.Slug == "":
		return ErrSlugBlank
	case !slugRe.MatchString(v.Slug):
		return ErrSlugInvalid
	case slugLen < 2:
		return ErrSlugTooShort
	case slugLen > 127:
		return ErrSlugTooLong
	}
	return nil
}
