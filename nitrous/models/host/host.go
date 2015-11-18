package host

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/securerandom"
)

var slugRe = regexp.MustCompile(`\A[a-z][a-z0-9\-]*[a-z0-9]\z`)

var (
	ErrSlugBlank    = errors.New("slug can't be blank")
	ErrSlugInvalid  = errors.New("slug may only contain alphanumeric characters or dashes and cannot begin or end with a dash")
	ErrSlugTooShort = errors.New("slug is too short (minimum is 2 characters)")
	ErrSlugTooLong  = errors.New("slug is too long (maximum is 63 characters)")
	ErrUpdateFailed = errors.New("failed to update host with stale original state")
)

const (
	StateBorn         = "born"
	StateRunning      = "running"
	StateStopped      = "stopped"
	StateStopping     = "stopping"
	StateStarting     = "starting"
	StateRestarting   = "restarting"
	StateUpdating     = "updating"
	StateOrphan       = "orphan"
	StateTerminating  = "terminating"
	StateTerminated   = "terminated"
	StateMigrating    = "migrating"
	StateRespeccing   = "respeccing"
	StateRespecFailed = "respec_failed"

	UpdateAvailableHard = "hard"
	UpdateAvailableSoft = "soft"
)

const (
	IDLE_NOTICES_NO_WARNING = iota
	IDLE_NOTICES_FIRST_WARNING
	IDLE_NOTICES_FINAL_WARNING
)

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
	FreighterdVersion string         `db:"freighterd_version"`
	DockerVersion     string         `db:"docker_version"`

	// The following fields are set using the response we get from
	// hostbuilder's creation pipeline and should be considered immutable for a
	// single host
	IPAddress        sql.NullString `db:"ip_address"`
	StaticIP         bool           `db:"static_ip"`
	FreighterdPort   sql.NullInt64  `db:"freighterd_port"`
	RegionIdentifier sql.NullString `db:"region_identifier"`
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

	// FIXME delete preprovisioned field and column
	Preprovisioned bool `db:"preprovisioned"`

	AssignedAt   *time.Time `db:"assigned_at"`
	TerminatedAt *time.Time `db:"terminated_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`

	// LastRunningAt records the last time a host became running, it is not reset when the host
	// is no longer running but rather serves as a marker for systems such as freetier.
	LastRunningAt pq.NullTime `db:"last_running_at"`

	// Tracks when a host has been upgraded
	UpgradedAt pq.NullTime `db:"upgraded_at"`

	// Tracks the last time at which ping webhook was called by freighter
	LastPingedAt pq.NullTime `db:"last_pinged_at"`
	AutoShutdown bool        `db:"auto_shutdown"`
	IdleTimeout  int64       `db:"idle_timeout"`

	Secret string `db:"secret"`

	// HourlyPrice unit is milli-cents (x1000)
	HourlyPrice    int64         `db:"hourly_price"`
	Free           bool          `db:"free"`
	SubscriptionID sql.NullInt64 `db:"subscription_id"`
	OrgID          sql.NullInt64 `db:"org_id"`

	// this is populated via a join
	SubscriptionSlug sql.NullString `db:"subscription_slug"`
	OrgSlug          sql.NullString `db:"org_slug"`

	*providertemplate.ProviderTemplate

	// Optional username field
	Username sql.NullString `db:"username"`

	ScanExempted  bool        `db:"scan_exempted"`
	NextScan      pq.NullTime `db:"next_scan"`
	TerminatingAt pq.NullTime `db:"terminating_at"`

	IdleNoticesSent int         `db:"idle_notices_sent"`
	IdleNextCheck   pq.NullTime `db:"idle_next_check"`

	Hotswap bool `db:"hotswap"`

	PendingRespec bool

	// Stroage Device ID ( VolumeID for amazon )
	ProviderStorageIdentifier sql.NullString `db:"provider_storage_identifier"`

	// this is determined externally but provided to the serializer
	Shared bool
}

func FindAllByProvider(db *db.Database, provider string) ([]*Host, error) {
	hosts := []*Host{}
	err := db.Select(&hosts, `SELECT hosts.*, username FROM hosts
		INNER JOIN provider_templates pt
			ON hosts.provider_template_id = pt.id
		INNER JOIN users
			ON users.id = hosts.user_id
		WHERE hosts.terminated_at IS NULL AND pt.provider = $1
		ORDER BY id;`, provider)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func FindAllByUser(db db.QueryReceiver, userID int64) ([]*Host, error) {
	hosts := []*Host{}
	err := db.Select(&hosts, `SELECT hosts.*, pt.provider, pt.config, sub.slug AS subscription_slug, orgs.slug AS org_slug
			FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
			LEFT JOIN subscriptions AS sub ON hosts.subscription_id = sub.id
			LEFT JOIN orgs ON hosts.org_id = orgs.id
			WHERE user_id = $1 AND terminated_at IS NULL
				AND preprovisioned = FALSE
				AND hosts.state NOT IN ($2, $3)
				AND (
					sub.state IS NULL OR -- users with no subscriptions still exist
					sub.state != $4 -- deleted subscriptions should not be visible to the user
				)
			ORDER BY id;`,
		userID, StateOrphan, StateMigrating, subscription.StateDeleted)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func FindAllWithTerminated(db *db.Database, userID int64) ([]*Host, error) {
	hosts := []*Host{}
	err := db.Select(&hosts, "SELECT * FROM hosts WHERE user_id = $1 AND preprovisioned = FALSE ORDER BY id;", userID)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func FindAllForTermination(db *db.Database) ([]*Host, error) {
	hosts := []*Host{}
	err := db.Select(&hosts, `SELECT hosts.*, pt.provider, pt.config, sub.slug AS subscription_slug
			FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
                        INNER JOIN subscriptions AS sub ON hosts.subscription_id = sub.id
                        WHERE sub.state = $1
                        AND sub.cancel_pending IS FALSE
                        AND hosts.terminated_at IS NULL
                        AND sub.cancelled_at < (now() - interval '48 hours');`, // 48hrs gives a grace period in case of any bugs (so we don't immediately term)
		subscription.StateDeleted)
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

// Find will return a Host scoped by User. Identifier can be either an ID or a slug.
// It also returns some data about the host's provider template.
func Find(db *db.Database, user *user.User, identifier string) (*Host, error) {
	host := &Host{}

	id, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		// If we can't parse an int, identifier is a slug.
		err = db.Get(host,
			`SELECT hosts.*, pt.provider, pt.config, sub.slug AS subscription_slug, orgs.slug AS org_slug
			FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
			LEFT JOIN subscriptions AS sub ON hosts.subscription_id = sub.id
                        LEFT JOIN orgs ON hosts.org_id = orgs.id
			WHERE hosts.user_id = $1 AND hosts.slug = $2
			AND terminated_at IS NULL AND preprovisioned = FALSE;`, user.ID, identifier)
	} else {
		err = db.Get(host,
			`SELECT hosts.*, pt.provider, pt.config, sub.slug AS subscription_slug, orgs.slug AS org_slug
			FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
			LEFT JOIN subscriptions AS sub ON hosts.subscription_id = sub.id
                        LEFT JOIN orgs ON hosts.org_id = orgs.id
			WHERE hosts.user_id = $1 AND hosts.id = $2
			AND terminated_at IS NULL AND preprovisioned = FALSE;`, user.ID, id)
	}

	if err != nil {
		return nil, err
	}

	return host, err
}

// FindShared will return a Host that has been shared via a container
// collaboration.
// TODO/Note: This should be changed to match container.FindShared (i.e. this
// func should also find hosts for which the given user is the owner)
func FindShared(db *db.Database, user *user.User, identifier string) (*Host, error) {
	host := &Host{}

	id, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		// If we can't parse an int, identifier is a slug.
		err = db.Get(host,
			`SELECT hosts.*, pt.provider, pt.config, sub.slug AS subscription_slug, orgs.slug AS org_slug
			FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
			INNER JOIN containers AS c ON hosts.id = c.host_id
			INNER JOIN container_collabs AS cc ON c.id = cc.container_id
			LEFT JOIN subscriptions AS sub ON hosts.subscription_id = sub.id
                        LEFT JOIN orgs ON hosts.org_id = orgs.id
			WHERE cc.user_id = $1 AND hosts.slug = $2
			AND hosts.terminated_at IS NULL AND preprovisioned = FALSE;`, user.ID, identifier)
	} else {
		err = db.Get(host,
			`SELECT hosts.*, pt.provider, pt.config, sub.slug AS subscription_slug, orgs.slug AS org_slug
			FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
			INNER JOIN containers AS c ON hosts.id = c.host_id
			INNER JOIN container_collabs AS cc ON c.id = cc.container_id
			LEFT JOIN subscriptions AS sub ON hosts.subscription_id = sub.id
                        LEFT JOIN orgs ON hosts.org_id = orgs.id
			WHERE cc.user_id = $1 AND hosts.id = $2
			AND hosts.terminated_at IS NULL AND preprovisioned = FALSE;`, user.ID, id)
	}

	if err != nil {
		return nil, err
	}

	return host, err
}

// FindByIdentifier returns a host identified by its ID or slug.
// It does not scope to a user and includes pre-provisioned hosts.
// It also returns some data about the host's provider template
func FindByIdentifier(db *db.Database, identifier string) (*Host, error) {
	host := &Host{}

	id, err := strconv.ParseInt(identifier, 10, 64)
	if err != nil {
		// If we can't parse an int, identifier is a slug.
		err = db.Get(host,
			`SELECT hosts.*, pt.provider, pt.config, sub.slug AS subscription_slug, orgs.slug AS org_slug
			FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
			LEFT JOIN subscriptions AS sub ON hosts.subscription_id = sub.id
                        LEFT JOIN orgs ON hosts.org_id = orgs.id
			WHERE hosts.slug = $1
			AND terminated_at IS NULL;`, identifier)
	} else {
		err = db.Get(host,
			`SELECT hosts.*, pt.provider, pt.config, sub.slug AS subscription_slug, orgs.slug AS org_slug
			FROM hosts
			INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
			LEFT JOIN subscriptions AS sub ON hosts.subscription_id = sub.id
                        LEFT JOIN orgs ON hosts.org_id = orgs.id
			WHERE hosts.id = $1
			AND terminated_at IS NULL;`, id)
	}

	if err != nil {
		return nil, err
	}

	return host, err
}

func FindBySubscriptionID(db *db.Database, subscriptionID int64) (*Host, error) {
	host := &Host{}
	err := db.Get(host,
		`SELECT hosts.* FROM hosts
		WHERE terminated_at IS NULL
		AND state <> 'terminated'
		AND subscription_id = $1;`, subscriptionID)
	return host, err
}

func (h Host) String() string {
	type h2 Host
	hs := h2(h)
	hs.Secret = "(REDACTED)"
	return fmt.Sprintf("%+v", hs)
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
	nstmt, err := d.PrepareNamed(`INSERT INTO hosts (
			user_id, org_id, region_id, plan_id, provider_template_id, slug, name, secret,
			state, ip_address, static_ip, provider_identifier, max_containers, size, freighterd_port,
			hourly_price, free, preprovisioned, freighterd_version, docker_version,
			region_identifier, last_pinged_at, auto_shutdown, hotswap
		) VALUES (
			:user_id, :org_id, :region_id, :plan_id, :provider_template_id, :slug, :name, :secret,
			:state, :ip_address, :static_ip, :provider_identifier, :max_containers, :size, :freighterd_port,
			:hourly_price, :free, :preprovisioned, :freighterd_version, :docker_version,
			:region_identifier, :last_pinged_at, :auto_shutdown, :hotswap
		)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE hosts
		SET (slug, name, state, ip_address, provider_identifier, max_containers, freighterd_port,
			preprovisioned, freighterd_version, docker_version, updated_at,
			region_identifier, last_pinged_at, auto_shutdown)
		= (:slug, :name, :state, :ip_address, :provider_identifier, :max_containers, :freighterd_port,
			:preprovisioned, :freighterd_version, :docker_version, now(),
			:region_identifier, :last_pinged_at, :auto_shutdown)
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) UpdateSpecifiedColumns(d db.QueryReceiver, columnNames []string) error {
	columnsCount := len(columnNames)

	columns := make([]string, 0, columnsCount)
	values := make([]string, 0, columnsCount)

	for _, columnName := range columnNames {
		columns = append(columns, fmt.Sprintf(`"%s"`, columnName))
		values = append(values, fmt.Sprintf(`:%s`, columnName))
	}

	query := fmt.Sprintf("UPDATE hosts SET (%s, updated_at) = (%s, now()) WHERE id = :id RETURNING *;", strings.Join(columns, ", "), strings.Join(values, ", "))
	nstmt, err := d.PrepareNamed(query)

	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) UpdateState(d db.QueryReceiver, newState string) error {
	err := d.QueryRowx(`UPDATE hosts
		SET state=$1, updated_at=now() WHERE id = $2 AND state=$3
		RETURNING *;`, newState, h.ID, h.State).StructScan(h)

	if err == sql.ErrNoRows {
		return ErrUpdateFailed
	}

	return err
}

func (h *Host) UpdateWithPrice(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE hosts
		SET (slug, name, state, ip_address, provider_identifier, max_containers, freighterd_port,
			preprovisioned, hourly_price, free, freighterd_version, docker_version, updated_at,
			region_identifier, last_pinged_at, auto_shutdown)
		= (:slug, :name, :state, :ip_address, :provider_identifier, :max_containers, :freighterd_port,
			:preprovisioned, :hourly_price, :free, :freighterd_version, :docker_version, now(),
			:region_identifier, :last_pinged_at, :auto_shutdown)
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) UpdateAfterRespec(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE hosts
		SET (provider_identifier, provider_storage_identifier,
			provider_template_id, state, ip_address, updated_at)
		= (:provider_identifier, :provider_storage_identifier,
			:provider_template_id, :state, :ip_address, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func (h *Host) UpdateVersions(d db.QueryReceiver, fv string, dv string) error {
	return d.QueryRowx(`UPDATE hosts SET freighterd_version = $1, docker_version = $2 WHERE id = $3 RETURNING *;`, fv, dv, h.ID).StructScan(h)
}

func (h *Host) UpdateColumn(d db.QueryReceiver, column string, value interface{}) error {
	// Warning: 2nd argument (column) should never be a user provided string!
	// Don't try update updated_at to specific time.
	return d.QueryRowx(fmt.Sprintf(`UPDATE hosts SET ("%s", updated_at) = ($1, now()) WHERE id = $2 RETURNING *;`, column), value, h.ID).StructScan(h)
}

func (h *Host) Reassign(d db.QueryReceiver, u *user.User) error {
	return d.QueryRowx(`UPDATE hosts SET user_id = $1, assigned_at = now() WHERE id = $2 RETURNING *;`, u.ID, h.ID).StructScan(h)
}

func (h *Host) Delete(d db.QueryReceiver) error {
	return d.QueryRowx(`UPDATE hosts SET terminated_at = now(), state = $1 WHERE id = $2 RETURNING *;`, StateTerminated, h.ID).StructScan(h)
}

func (h *Host) Touch(d db.QueryReceiver) error {
	return h.UpdateColumn(d, "last_pinged_at", pq.NullTime{time.Now(), true})
}

func (h *Host) CanCreateMoreContainers(d db.QueryReceiver, u *user.User) (bool, error) {
	if !h.MaxContainers.Valid && !u.MaxContainers.Valid {
		return true, nil
	}

	containerLimit := h.MaxContainers
	if !containerLimit.Valid || containerLimit.Int64 < u.MaxContainers.Int64 {
		containerLimit = u.MaxContainers
	}
	if !containerLimit.Valid {
		return true, nil
	}

	var count int64
	err := d.QueryRowx(`SELECT COUNT(*) FROM containers WHERE host_id = $1 AND terminated_at IS NULL;`, h.ID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count < containerLimit.Int64, nil
}

func (h *Host) Validate() error {
	if h.Slug == "" {
		return ErrSlugBlank
	}
	slugLen := len(h.Slug)
	if slugLen < 2 {
		return ErrSlugTooShort
	}
	if slugLen > 63 {
		return ErrSlugTooLong
	}
	if !slugRe.MatchString(h.Slug) {
		return ErrSlugInvalid
	}
	return nil
}

// MachineConfig returns the configuration that hostbuilder can use to
// identify the host and manipulate it.
func (h *Host) MachineConfig() hostbuilderclient.Config {
	cfg := hostbuilderclient.Config{
		"host_id":    h.ID,
		"id":         h.ProviderIdentifier.String,
		"volume_id":  h.ProviderStorageIdentifier.String,
		"region_id":  h.RegionIdentifier.String,
		"ip_address": h.IPAddress.String,
		"hotswap":    h.Hotswap,
	}

	if pt := h.ProviderTemplate; pt != nil {
		for k, v := range pt.Config.Map {
			if v.Valid {
				cfg[k] = v.String
			}
		}

		switch pt.Provider {
		case "amazon":
			// hostbuilder will replace the {{env}} with the hostbuilder environment
			cfg["name"] = fmt.Sprintf("userhost-{{env}}-%d", h.ID)

			size, err := strconv.Atoi(cfg["storage_size"].(string))
			if err == nil {
				cfg["storage_size"] = size
			}

			var disableEIP bool
			if v, ok := pt.Config.Map["disable_eip"]; ok {
				disableEIP, _ = strconv.ParseBool(v.String)
			}
			cfg["disable_eip"] = disableEIP
		default:
			// Don't care about other providers.
		}
	}

	return cfg
}

// Hostname returns the public domain that the host can be reached. This should be the
// same as the DNS address which we map the the host IP address
//
// We assume that slug is vald ( its cant be empty ), therefore we wont check
// it here
func (h *Host) Hostname() string {
	return fmt.Sprintf("%s.%s", h.Slug, envvars.HostHostname)
}

func (h *Host) CertHostname() string {
	return fmt.Sprintf("*.%s", envvars.HostHostname)
}

func (h *Host) ProxyHostname() string {
	return fmt.Sprintf("%s.%s", h.Slug, envvars.ProxyHostname)
}

func URLArgsFromHost(h *Host) url.Values {
	v := url.Values{}
	v.Set("id", fmt.Sprintf("%d", h.ID))
	v.Set("uid", fmt.Sprintf("%d", h.UserID))
	v.Set("slug", h.Slug)
	v.Set("queue", "true")
	return v
}
