package org

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
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

	ErrMaxHostsReached = errors.New("reached maximum number of hosts")
	ErrNoCard          = errors.New("has not added credit card")
	ErrDelinquent      = errors.New("outstanding invoices")
)

type Org struct {
	ID    int64  `db:"id"`
	Slug  string `db:"slug"`
	Name  string `db:"name"`
	Email string `db:"email"`

	Edu bool `db:"edu"`

	MaxHosts sql.NullInt64 `db:"max_hosts"`

	// Track number of active hosts belonging to org.
	UsedHosts int

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`

	DeletedAt pq.NullTime `db:"deleted_at"`
}

type OrgWithRole struct {
	*Org

	Role string `db:"role"`
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

func FindAllByMemberUserID(d db.QueryReceiver, uid int64) ([]*OrgWithRole, error) {
	orgs := []*OrgWithRole{}
	err := d.Select(&orgs, `
		SELECT orgs.*, org_memberships.role
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

	if _, err := strconv.ParseInt(o.Slug, 10, 64); err == nil {
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

func (o *Org) CanCreateHosts(d db.QueryReceiver) error {
	bp, err := billingprofile.FindForOrg(d, o.ID)
	if err != nil {
		return err
	}
	if err = bp.CanCreateHosts(); err != nil {
		switch err {
		case billingprofile.ErrNoCard:
			return ErrNoCard
		case billingprofile.ErrDelinquent:
			return ErrDelinquent
		default:
			return err
		}
	}

	if !o.MaxHosts.Valid {
		return nil
	}

	var b bool
	if err = d.QueryRowx(`SELECT (count(h.id) < coalesce(min(o.max_hosts), 0))
		FROM orgs o
		LEFT JOIN hosts h ON (h.org_id = o.id AND h.terminated_at IS NULL)
		WHERE o.id = $1;`,
		o.ID).Scan(&b); err != nil {
		return err
	}
	if !b {
		return ErrMaxHostsReached
	}

	return nil
}

func (o *Org) UsedHostsCount(d *db.Database) (int, error) {
	orgs, err := host.FindAllByOrgID(d, o.ID)
	return len(orgs), err
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
