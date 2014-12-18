package host

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

var slugRe = regexp.MustCompile(`\A[a-z0-9][a-z0-9\-]*[a-z0-9]\z`)

var (
	ErrSlugBlank    = errors.New("slug can't be blank")
	ErrSlugInvalid  = errors.New("slug may only contain alphanumeric characters or dashes and cannot begin or end with a dash")
	ErrSlugTooShort = errors.New("slug is too short (minimum is 2 characters)")
	ErrSlugTooLong  = errors.New("slug is too long (maximum is 63 characters)")
)

type ProviderTemplate struct {
	Provider string `db:"provider"`
}

const (
	StateBorn       = "born"
	StateRunning    = "running"
	StateStopped    = "stopped"
	StateStopping   = "stopping"
	StateStarting   = "starting"
	StateRestarting = "restarting"
	StateOrphan     = "orphan"
	StateTerminated = "terminated"
)

// slug is prefixed to these as "<slug>.<rootHostname>" and "<slug>.<proxyHostname>"
var rootHostname, proxyHostname string

func init() {
	rootHostname = os.Getenv("HOST_HOSTNAME")
	if rootHostname == "" {
		log.Fatalf("HOST_HOSTNAME not set")
	}
	proxyHostname = os.Getenv("PROXY_HOSTNAME")
	if proxyHostname == "" {
		log.Fatalf("PROXY_HOSTNAME not set")
	}
}

type Host struct {
	ID                 int64 `db:"id"`
	UserID             int64 `db:"user_id"`
	RegionID           int64 `db:"region_id"`
	PlanID             int64 `db:"plan_id"`
	ProviderTemplateID int64 `db:"provider_template_id"`

	// Slug is used as a DNS label in the host's FQDN (thus limited to 63 chars).
	Slug              string         `db:"slug"`
	Name              sql.NullString `db:"name"`
	State             string         `db:"state"`
	IPAddress         sql.NullString `db:"ip_address"`
	FreighterdPort    sql.NullInt64  `db:"freighterd_port"`
	FreighterdVersion string         `db:"freighterd_version"`
	DockerVersion     string         `db:"docker_version"`

	// ProviderIdentifier is the identifier used by the host's provider to
	// uniquely identify this host.
	ProviderIdentifier sql.NullString `db:"provider_identifier"`

	// MaxContainers is the maximum number of containers that can be created on
	// this host. This is copied over from the associated Plan so that it can
	// be changed independently when necessary.
	MaxContainers sql.NullInt64 `db:"max_containers"`

	// Size is currently the name of the plan of this host. This is copied over
	// from the associated Plan since size is often read.
	Size sql.NullString `db:"size"`

	Preprovisioned bool `db:"preprovisioned"`

	AssignedAt   *time.Time `db:"assigned_at"`
	TerminatedAt *time.Time `db:"terminated_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`

	Secret string `db:"secret"`

	// HourlyPrice unit is milli-cents (x1000)
	HourlyPrice int64 `db:"hourly_price"`

	*ProviderTemplate
}

func FindAllByUser(db *db.Database, user *user.User) ([]Host, error) {
	hosts := []Host{}
	err := db.Select(&hosts, "SELECT * FROM hosts WHERE user_id = $1 AND terminated_at IS NULL AND preprovisioned = FALSE ORDER BY id;", user.ID)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func FindAllWithTerminated(db *db.Database, user *user.User) ([]Host, error) {
	hosts := []Host{}
	err := db.Select(&hosts, "SELECT * FROM hosts WHERE user_id = $1 AND preprovisioned = FALSE ORDER BY id;", user.ID)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func CountPreprovisioned(db *db.Database, r *region.Region, p *plan.Plan) (int, error) {
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM hosts
		WHERE region_id = $1 AND plan_id = $2 AND state = $3
		AND preprovisioned = TRUE AND terminated_at IS NULL;`,
		r.ID, p.ID, StateRunning).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func CountPreprovisionedOrphans(db *db.Database, r *region.Region) (int, error) {
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM hosts
		WHERE region_id = $1 AND state = $2
		AND preprovisioned = TRUE AND terminated_at IS NULL;`,
		r.ID, StateOrphan).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// Find will return a Host scoped by User.
// identifier can be either an ID or a slug.
func Find(db *db.Database, user *user.User, identifier string) (*Host, error) {
	host := &Host{}

	id, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		// Parsing resulted in an error.
		// That means identifier is a slug
		err = db.Get(host,
			`SELECT hosts.*, provider_templates.provider
			FROM hosts
			INNER JOIN provider_templates ON hosts.provider_template_id = provider_templates.id
			WHERE hosts.user_id = $1 AND hosts.slug = $2
			AND terminated_at IS NULL AND preprovisioned = FALSE;`, user.ID, identifier)
	} else {
		err = db.Get(host,
			`SELECT hosts.*, provider_templates.provider
			FROM hosts
			INNER JOIN provider_templates ON hosts.provider_template_id = provider_templates.id
			WHERE hosts.user_id = $1 AND hosts.id = $2
			AND terminated_at IS NULL AND preprovisioned = FALSE;`, user.ID, id)
	}

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return host, err
}

func FindByID(db *db.Database, id int64) (*Host, error) {
	host := &Host{}
	err := db.Get(host, `SELECT * FROM hosts WHERE id = $1`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return host, err
}

func FindBySlug(db *db.Database, slug string) (*Host, error) {
	host := &Host{}
	q := `SELECT hosts.*, provider_templates.provider
				FROM hosts
				INNER JOIN provider_templates ON hosts.provider_template_id = provider_templates.id
				WHERE hosts.slug = $1 AND terminated_at IS NULL AND preprovisioned = FALSE;`
	err := db.Get(host, q, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return host, err
}

func FindPreprovisioned(db *db.Database, r *region.Region, p *plan.Plan) (*Host, error) {
	host := &Host{}
	if err := db.Get(host,
		`SELECT * FROM hosts
		WHERE region_id = $1 AND plan_id = $2 AND state = $3
		AND preprovisioned = TRUE AND terminated_at IS NULL ORDER BY id;`,
		r.ID, p.ID, StateRunning); err != nil {
		return nil, err
	}
	return host, nil
}

func (h *Host) Insert(d db.QueryReceiver) error {
	if h.State == "" {
		h.State = StateBorn
	}
	if h.Secret == "" {
		sec, err := securerandom.Base64(191)
		if err != nil {
			return err
		}
		h.Secret = sec[:255]
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO hosts
		(user_id, region_id, plan_id, provider_template_id, slug, name, secret,
			state, ip_address, provider_identifier, max_containers, size, freighterd_port,
			hourly_price, preprovisioned, freighterd_version, docker_version)
		VALUES
		(:user_id, :region_id, :plan_id, :provider_template_id, :slug, :name, :secret,
		:state, :ip_address, :provider_identifier, :max_containers, :size, :freighterd_port,
		:hourly_price, :preprovisioned, :freighterd_version, :docker_version)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE hosts
		SET (slug, name, state, ip_address, provider_identifier, max_containers, freighterd_port, preprovisioned, hourly_price, updated_at)
		= (:slug, :name, :state, :ip_address, :provider_identifier, :max_containers, :freighterd_port, :preprovisioned, :hourly_price, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) Reassign(d db.QueryReceiver, u *user.User) error {
	return d.QueryRowx(`UPDATE hosts SET user_id = $1, assigned_at = now() WHERE id = $2 RETURNING *`, u.ID, h.ID).StructScan(h)
}

func (h *Host) Delete(d db.QueryReceiver) error {
	return d.QueryRowx(`UPDATE hosts SET terminated_at = now(), state = $1 WHERE id = $2 RETURNING *;`, StateTerminated, h.ID).StructScan(h)
}

func (h *Host) Validate() error {
	if h.Slug == "" {
		return ErrSlugBlank
	}
	nameLen := len(h.Slug)
	if nameLen < 2 {
		return ErrSlugTooShort
	}
	if nameLen > 63 {
		return ErrSlugTooLong
	}
	if !slugRe.MatchString(h.Slug) {
		return ErrSlugInvalid
	}
	return nil
}

// Hostname returns the public domain that the host can be reached. This should be the
// same as the DNS address which we map the the host IP address
//
// We assume that slug is vald ( its cant be empty ), therefore we wont check
// it here
func (h *Host) Hostname() string {
	return fmt.Sprintf("%s.%s", h.Slug, rootHostname)
}

func (h *Host) CertHostname() string {
	return fmt.Sprintf("*.%s", rootHostname)
}

func (h *Host) ProxyHostname() string {
	return fmt.Sprintf("%s.%s", h.Slug, proxyHostname)
}
