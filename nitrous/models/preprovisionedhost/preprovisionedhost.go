package preprovisionedhost

import (
	"database/sql"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
)

type PreprovisionedHost struct {
	ID                 int64          `db:"id"`
	RegionID           int64          `db:"region_id"`
	PlanID             int64          `db:"plan_id"`
	ProviderTemplateID int64          `db:"provider_template_id"`
	Slug               string         `db:"slug"`
	State              string         `db:"state"`
	IPAddress          sql.NullString `db:"ip_address"`
	FreighterdPort     sql.NullInt64  `db:"freighterd_port"`
	Secret             string         `db:"secret"`
	ProviderIdentifier sql.NullString `db:"provider_identifier"`

	TerminatedAt *time.Time `db:"terminated_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`
}

func FindAll(db *db.Database, r *region.Region, p *plan.Plan) ([]*PreprovisionedHost, error) {
	hosts := []*PreprovisionedHost{}
	if err := db.Select(&hosts, `SELECT * FROM preprovisioned_hosts WHERE region_id = $1 AND plan_id = $2 AND terminated_at IS NULL ORDER BY id;`, r.ID, p.ID); err != nil {
		return nil, err
	}
	return hosts, nil
}
