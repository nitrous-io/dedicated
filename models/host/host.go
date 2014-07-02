package host

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/user"
)

type Host struct {
	ID                 int64 `db:"id"`
	UserID             int64 `db:"user_id"`
	RegionID           int64 `db:"region_id"`
	PlanID             int64 `db:"plan_id"`
	ProviderTemplateID int64 `db:"provider_template_id"`

	// Name is the hostname and is used as a DNS label in a FQDN (thus limited
	// to 63 characters).
	Name      string         `db:"name"`
	LongName  sql.NullString `db:"long_name"`
	State     string         `db:"state"`
	IPAddress sql.NullString `db:"ip_address"`

	// ProviderIdentifier is the identifier used by the host's provider to
	// uniquely identify this host.
	ProviderIdentifier sql.NullString `db:"provider_identifier"`

	// MaxContainers is the maximum number of containers that can be created on
	// this host. This is copied over from the associated Plan so that it can
	// be changed independently when necessary.
	MaxContainers sql.NullInt64 `db:"max_containers"`

	TerminatedAt *time.Time `db:"terminated_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`
}

func FindAllByUser(db *db.Database, user *user.User) ([]Host, error) {
	return findAllByUserID(db, user.ID)
}

func findAllByUserID(db *db.Database, userID int64) ([]Host, error) {
	hosts := []Host{}
	err := db.Select(&hosts, "SELECT * from hosts WHERE user_id = $1 ORDER BY id", userID)
	if err != nil {
		return []Host{}, err
	}
	return hosts, nil
}

func (h *Host) Insert(d db.QueryReceiver) error {
	_, err := d.NamedExec(`INSERT INTO hosts
		(user_id, region_id, plan_id, provider_template_id,
			name, long_name, state,
			ip_address, provider_identifier, max_containers)
		VALUES
		(:user_id, :region_id, :plan_id, :provider_template_id,
			:name, :long_name, :state,
			:ip_address, :provider_identifier, :max_containers);`, h)
	if err != nil {
		return err
	}
	return d.Get(h, "SELECT * FROM hosts WHERE user_id = $1 AND name = $2;", h.UserID, h.Name)
}

func (h *Host) Validate() error {
	// name should be unique since the API accepts hostname as a host identifier.
	return nil
}
