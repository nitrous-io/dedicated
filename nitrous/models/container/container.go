package container

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

// must match freighter/provider.containerNameRe
var slugRe = regexp.MustCompile(`\A[a-z][a-z0-9\-]*[a-z0-9]\z`)

var (
	ErrSlugBlank    = errors.New("slug can't be blank")
	ErrSlugInvalid  = errors.New("slug may only contain alphanumeric characters or dashes and must begin with a letter")
	ErrSlugTooShort = errors.New("slug is too short (minimum is 2 characters)")
	ErrSlugTooLong  = errors.New("slug is too long (maximum is 63 characters)")
	ErrImageBlank   = errors.New("image can't be blank")
	ErrLocked       = errors.New("container is locked")
)

type Container struct {
	ID     int64          `db:"id"`
	CID    string         `db:"cid"`
	UserID int64          `db:"user_id"`
	HostID int64          `db:"host_id"`
	Image  string         `db:"image"`
	Slug   string         `db:"slug"`
	Name   sql.NullString `db:"name"`
	Secret string         `db:"secret"`

	// TempState can be used to overlay the actual container state.
	// ResolvedState() returns this value when set.
	// Eg: When TempState is set to 'starting` and container state in Freighter is 'stopped',
	// ResolvedState() would return 'starting'.
	TempState sql.NullInt64 `db:"temp_state"`

	Metadata hstore.Hstore `db:"metadata"`

	TerminatedAt *time.Time `db:"terminated_at"`
	TouchedAt    *time.Time `db:"touched_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`
	LockedUntil  *time.Time `db:"locked_until"`

	PortMappings string `db:"port_mappings"`

	// These fields come from Freighter.
	State      State
	User       string
	IPAddress  string
	GatewayIP  string
	Cmd        []string
	Entrypoint []string
	ExitCode   int
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// Find queries for a container scoped to the user.
// identifier can be either an ID or a slug.
func Find(d db.QueryReceiver, user *user.User, identifier string) (*Container, error) {
	c := &Container{}
	id, err := strconv.ParseInt(identifier, 10, 64)

	if err != nil {
		err = d.Get(c,
			`SELECT * FROM containers WHERE user_id = $1 AND slug = $2 AND containers.terminated_at IS NULL;`, user.ID, identifier)
	} else {
		err = d.Get(c,
			`SELECT * FROM containers WHERE user_id = $1 AND id = $2 AND containers.terminated_at IS NULL;`, user.ID, id)
	}

	if err != nil {
		return nil, err
	}
	return c, nil
}

// FindShared queries for a container where given user is either the owner or a
// collaborator.  identifier can be either an ID or a slug.
func FindShared(d db.QueryReceiver, user *user.User, identifier string) (*Container, error) {
	c := &Container{}
	id, err := strconv.ParseInt(identifier, 10, 64)

	if err != nil {
		err = d.Get(c,
			`SELECT
			containers.*
			FROM containers
			LEFT JOIN container_collabs ON containers.id = container_collabs.container_id
			WHERE (container_collabs.user_id = $1 OR containers.user_id = $1)
			AND slug = $2
			AND containers.terminated_at IS NULL;`,
			user.ID, identifier)
	} else {
		err = d.Get(c,
			`SELECT
			containers.*
			FROM containers
			LEFT JOIN container_collabs ON containers.id = container_collabs.container_id
			WHERE (container_collabs.user_id = $1 OR containers.user_id = $1)
			AND containers.id = $2
			AND containers.terminated_at IS NULL;`,
			user.ID, id)
	}

	if err != nil {
		return nil, err
	}
	return c, nil
}

func FindAllShared(d db.QueryReceiver, userID int64) ([]*Container, error) {
	containers := []*Container{}
	err := d.Select(&containers, `
		SELECT containers.* FROM containers
		INNER JOIN container_collabs ON container_collabs.container_id = containers.id
		WHERE containers.terminated_at IS NULL
		AND container_collabs.user_id = $1;`,
		userID)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func FindByCID(d db.QueryReceiver, cid string) (*Container, error) {
	c := &Container{}
	err := d.Get(c, `SELECT * FROM containers WHERE cid = $1 AND terminated_at IS NULL;`, cid)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func FindAllByHostID(d db.QueryReceiver, hostID int64) ([]*Container, error) {
	containers := []*Container{}
	err := d.Select(&containers, `SELECT * FROM containers WHERE host_id = $1 AND terminated_at IS NULL;`, hostID)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func FindAllByHostIDs(d db.QueryReceiver, hostIDs []int64) ([]*Container, error) {
	containers := []*Container{}

	// sqlx currently does not like empty slices for In
	if len(hostIDs) == 0 {
		return containers, nil
	}

	sql, args, err := sqlx.In("SELECT * FROM containers WHERE host_id IN (?) AND terminated_at IS NULL;", hostIDs)
	if err != nil {
		return nil, err
	}

	sql = d.Rebind(sql)
	err = d.Select(&containers, sql, args...)
	if err != nil {
		return nil, err
	}
	return containers, nil
}

func DeleteAllByHostID(d db.QueryReceiver, hostID int64) error {
	_, err := d.Exec(`UPDATE containers SET terminated_at = now() WHERE host_id = $1;`, hostID)
	return err
}

func (c Container) String() string {
	type c2 Container
	cs := c2(c)
	cs.Secret = "(REDACTED)"
	return fmt.Sprintf("%+v", cs)
}

func (c *Container) Insert(d db.QueryReceiver) error {
	if c.Secret == "" {
		sec, err := securerandom.Base64(191)
		if err != nil {
			return err
		}
		c.Secret = sec[:255]
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO containers
		(cid, user_id, host_id, image, slug, name, secret, metadata)
    VALUES (:cid, :user_id, :host_id, :image, :slug, :name, :secret, :metadata)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}

func (c *Container) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE containers
		SET (cid, slug, name, image, temp_state, touched_at, updated_at, port_mappings)
		= (:cid, :slug, :name, :image, :temp_state, :touched_at, now(), :port_mappings)
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}

func (c *Container) UpdateColumn(d db.QueryReceiver, column string, value interface{}) error {
	// Warning: 2nd argument (column) should never be a user provided string!
	_, err := d.Exec(fmt.Sprintf(`UPDATE "containers" SET ("%s", updated_at) = ($1, now()) WHERE id = $2;`, column), value, c.ID)
	return err
}

func (c *Container) Delete(d db.QueryReceiver) error {
	// caller expected to use transaction if necessary
	if _, err := d.Exec(`DELETE FROM container_volumes WHERE container_id = $1`, c.ID); err != nil {
		return err
	}
	nstmt, err := d.PrepareNamed(`UPDATE containers SET terminated_at = now() WHERE id = :id RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}

func (c *Container) Validate() error {
	if c.Slug == "" {
		return ErrSlugBlank
	}
	nameLen := len(c.Slug)
	if nameLen < 2 {
		return ErrSlugTooShort
	}
	if nameLen > 63 {
		return ErrSlugTooLong
	}
	if !slugRe.MatchString(c.Slug) {
		return ErrSlugInvalid
	}
	if c.Image == "" {
		return ErrImageBlank
	}
	return nil
}

func (c *Container) SetState(state string) bool {
	var ok bool
	switch state {
	case "running":
		c.State, ok = StateRunning, true
	case "stopped":
		c.State, ok = StateStopped, true
	}
	return ok
}

func (c *Container) ResolvedState() State {
	if c.TerminatedAt != nil {
		return StateTerminated
	}

	if c.TempState.Valid && c.IsLocked() {
		return State(c.TempState.Int64)
	}
	return c.State
}

func (c *Container) IsLocked() bool {
	if c.LockedUntil == nil {
		return false
	} else {
		return time.Now().Before(*c.LockedUntil)
	}
}

func (c *Container) Lock(state State, expiresIn time.Duration, d db.QueryReceiver) error {
	now := time.Now()
	expiredAt := now.Add(expiresIn)

	res, err := d.Exec(`
		UPDATE containers SET (temp_state, locked_until) = ($1, $2) WHERE id = $3 AND (locked_until IS NULL OR locked_until < $4)
	`, int64(state), expiredAt, c.ID, now)

	if err != nil {
		return err
	}

	i, _ := res.RowsAffected()
	if i < 1 {
		return ErrLocked
	}
	c.LockedUntil = &expiredAt
	c.TempState = sql.NullInt64{int64(state), true}
	return nil
}

func (c *Container) Unlock(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE containers SET (temp_state, locked_until) = (null, null) WHERE id = :id RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(c).StructScan(c)
}

func (c *Container) PreviewHostname() string {
	return fmt.Sprintf("%s.%s", c.Slug, envvars.PreviewHostname)
}
