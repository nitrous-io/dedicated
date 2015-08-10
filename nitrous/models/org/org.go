package org

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/pkg/blacklist"
	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/customer"
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

	MaxHosts int64 `db:"max_hosts"`

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

func Find(d db.QueryReceiver, id int64) (*Org, error) {
	org := &Org{}
	err := d.Get(org, "SELECT * FROM orgs WHERE id = $1 AND deleted_at IS NULL", id)
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

func (o *Org) CanCreateHosts(d db.QueryReceiver) (bool, error) {
	bp, err := billingprofile.FindForOrg(d, o.ID)
	if err != nil {
		return false, err
	}

	if !bp.CanCreateHosts() {
		return false, nil
	}

	// ensure max_hosts limit has not been reached
	var b bool
	err = d.QueryRowx(`SELECT (count(h.id) < coalesce(min(o.max_hosts), 0))
		FROM orgs o
		LEFT JOIN hosts h ON (h.org_id = o.id AND h.terminated_at IS NULL)
		WHERE o.id = $1;`,
		o.ID).Scan(&b)
	if err != nil {
		return false, err
	}

	return b, nil
}

func (o *Org) ActivateBillingProfile(d *db.Database, bp *billingprofile.BillingProfile, token string) (err error) {
	params := &stripe.CustomerParams{
		Email: o.Email,
		Desc:  fmt.Sprintf("Organization %q", o.Name),
		Params: stripe.Params{
			Meta: map[string]string{
				"name": o.Name,
			},
		},
	}
	if token != "" {
		params.Token = token
	}
	c, err := customer.New(params)
	if err != nil {
		err = billingprofile.HandleStripeError(err)
		return
	}

	defer func() {
		if err != nil {
			if err := customer.Del(c.ID); err != nil {
				logrus.Errorf("Unable to delete customer %s, err: %v", c.ID, err)
			}
		}
	}()

	err = bp.Activate(d, c)
	return
}
