package org

import (
	"errors"
	"regexp"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/pkg/blacklist"
)

var (
	slugRe  = regexp.MustCompile(`\A[a-z0-9][a-z0-9\-]*[a-z0-9]\z`)
	emailRe = regexp.MustCompile(`\A[^@\s]+@([^@\s]+\.)+[^@\s]+\z`)
)

var (
	ErrSlugBlank        = errors.New("slug can't be blank")
	ErrSlugInvalid      = errors.New("slug may only contain alphanumeric characters or dashes and cannot begin or end with a dash")
	ErrSlugTooShort     = errors.New("slug is too short (minimum is 2 characters)")
	ErrSlugTooLong      = errors.New("slug is too long (maximum is 63 characters)")
	ErrEmailBlank       = errors.New("email can't be blank")
	ErrEmailInvalid     = errors.New("email is not valid")
	ErrEmailBlacklisted = errors.New("email is blacklisted")
	ErrNameBlank        = errors.New("name can't be blank")
)

type Org struct {
	ID    int64  `db:"id"`
	Slug  string `db:"slug"`
	Name  string `db:"name"`
	Email string `db:"email"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`

	DeletedAt pq.NullTime `db:"deleted_at"`
}

func FindBySlug(d db.QueryReceiver, slug string) (*Org, error) {
	org := &Org{}
	err := d.Get(org, "SELECT * FROM orgs WHERE slug = $1 AND deleted_at IS NULL", slug)
	if err != nil {
		return nil, err
	}
	return org, nil
}

func FindAllByMemberUserID(d db.QueryReceiver, uid int64) ([]*Org, error) {
	orgs := []*Org{}
	err := d.Select(&orgs, `
		SELECT orgs.*
		FROM orgs
		INNER JOIN org_memberships ON orgs.id = org_memberships.org_id
		WHERE org_memberships.user_id = $1 AND orgs.deleted_at IS NULL
		ORDER BY orgs.id;
	`, uid)
	if err != nil {
		return nil, err
	}
	return orgs, nil
}

func (o *Org) Insert(d db.QueryReceiver) error {
	s, err := d.PrepareNamed(`
		INSERT INTO orgs
		(slug, name, email)
		VALUES (:slug, :name, :email)
		RETURNING *;
	`)
	if err != nil {
		return err
	}
	return s.QueryRowx(o).StructScan(o)
}

func (o *Org) Validate() error {
	if o.Slug == "" {
		return ErrSlugBlank
	}

	slugLen := len(o.Slug)
	if slugLen < 2 {
		return ErrSlugTooShort
	}
	if slugLen > 63 {
		return ErrSlugTooLong
	}

	if !slugRe.MatchString(o.Slug) {
		return ErrSlugInvalid
	}

	if o.Email == "" {
		return ErrEmailBlank
	}
	if !emailRe.MatchString(o.Email) {
		return ErrEmailInvalid
	}
	if blacklist.Check(o.Email) {
		return ErrEmailBlacklisted
	}

	if o.Name == "" {
		return ErrNameBlank
	}

	return nil
}
