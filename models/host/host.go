package host

import (
	"database/sql"
	"errors"
	"regexp"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
)

var (
	nameRe = regexp.MustCompile(`\A[a-z0-9][a-z0-9\-]*\z`)
)

var (
	ErrSlugBlank    = errors.New("slug can't be blank")
	ErrSlugInvalid  = errors.New("slug is not valid")
	ErrSlugTooShort = errors.New("slug is too short (minimum is 2 characters)")
	ErrSlugTooLong  = errors.New("slug is too long (maximum is 63 characters)")
)

type ProviderTemplate struct {
	Provider string `db:"provider"`
}

const (
	StateCreated    = "created"
	StateRunning    = "running"
	StateStopped    = "stopped"
	StateStopping   = "stopping"
	StateStarting   = "starting"
	StateRestarting = "restarting"
)

type Host struct {
	ID                 int64 `db:"id"                   json:"id"`
	UserID             int64 `db:"user_id"              json:"user_id"`
	RegionID           int64 `db:"region_id"            json:"region_id"`
	PlanID             int64 `db:"plan_id"              json:"plan_id"`
	ProviderTemplateID int64 `db:"provider_template_id" json:"provider_template_id"`

	// Name is the hostname and is used as a DNS label in a FQDN (thus limited
	// to 63 characters).
	Slug      string         `db:"slug"       json:"slug"`
	Name      sql.NullString `db:"name"       json:"name"`
	State     string         `db:"state"      json:"state"`
	IPAddress sql.NullString `db:"ip_address" json:"ip_address"`

	// ProviderIdentifier is the identifier used by the host's provider to
	// uniquely identify this host.
	ProviderIdentifier sql.NullString `db:"provider_identifier" json:"provider_identifier"`

	// MaxContainers is the maximum number of containers that can be created on
	// this host. This is copied over from the associated Plan so that it can
	// be changed independently when necessary.
	MaxContainers sql.NullInt64 `db:"max_containers" json:"provider_identifier"`

	// Size is currently the name of the plan of this host. This is copied over
	// from the associated Plan since size is often read.
	Size db.NullString `db:"size" json:"size"`

	// FreighterdPort is needed during dev, since port will be different everytime you
	// create a host.
	FreighterdPort sql.NullInt64 `db:"freighterd_port" json:"freighterd_port"`

	TerminatedAt *time.Time `db:"terminated_at" json:"terminated_at"`
	CreatedAt    *time.Time `db:"created_at"    json:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"    json:"updated_at"`

	*ProviderTemplate
}

func FindAllByUser(db *db.Database, user *user.User) ([]Host, error) {
	return findAllByUserID(db, user.ID)
}

func findAllByUserID(db *db.Database, userID int64) ([]Host, error) {
	hosts := []Host{}
	err := db.Select(&hosts, "SELECT * FROM hosts WHERE user_id = $1 AND terminated_at IS NULL ORDER BY id;", userID)
	if err != nil {
		return nil, err
	}
	return hosts, nil
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

func FindByUserAndSlug(db *db.Database, user *user.User, slug string) (*Host, error) {
	host := &Host{}
	q := `SELECT hosts.*, provider_templates.provider
				FROM hosts
				INNER JOIN provider_templates ON hosts.provider_template_id = provider_templates.id
				WHERE hosts.user_id = $1 AND hosts.slug = $2 AND terminated_at IS NULL;`
	err := db.Get(host, q, user.ID, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return host, err
}

func (h *Host) Insert(d db.QueryReceiver) error {
	if h.State == "" {
		h.State = "born"
	}
	nstmt, err := d.PrepareNamed(`INSERT INTO hosts
		(user_id, region_id, plan_id, provider_template_id, slug, name,
			state, ip_address, provider_identifier, max_containers, size, freighterd_port)
		VALUES
		(:user_id, :region_id, :plan_id, :provider_template_id, :slug,
		:name, :state, :ip_address, :provider_identifier, :max_containers, :size, :freighterd_port)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) UpdateState(d db.QueryReceiver, state string) error {
	h.State = state
	_, err := d.Exec("UPDATE hosts SET state = $1 WHERE id = $2", state, h.ID)
	return err
}

func (h *Host) Delete(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE hosts SET terminated_at = now() WHERE id = :id RETURNING *;`)
	if err != nil {
		return nil
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) Validate(db *db.Database) error {
	if h.Slug == "" {
		return ErrSlugBlank
	}
	if !nameRe.MatchString(h.Slug) {
		return ErrSlugInvalid
	}
	nameLen := len(h.Slug)
	if nameLen < 2 {
		return ErrSlugTooShort
	}
	if nameLen > 63 {
		return ErrSlugTooLong
	}
	return nil
}
