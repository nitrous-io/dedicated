package providertemplate

import (
	"time"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
)

type ProviderTemplate struct {
	ID                int64         `db:"id"`
	RegionID          int64         `db:"region_id"`
	PlanID            int64         `db:"plan_id"`
	Name              string        `db:"name"`
	Provider          string        `db:"provider"`
	Config            hstore.Hstore `db:"config"`
	PreprovisionMax   int           `db:"preprovision_max"`
	SupportsStopStart bool          `db:"supports_start_stop"`
	Disabled          bool          `db:"disabled"`
	CreatedAt         *time.Time    `db:"created_at"`
	UpdatedAt         *time.Time    `db:"updated_at"`
}

func Preprovisioned(d db.QueryReceiver) ([]ProviderTemplate, error) {
	pts := []ProviderTemplate{}
	if err := d.Select(&pts, `SELECT * FROM provider_templates WHERE preprovision_max > 0`); err != nil {
		return nil, err
	}
	return pts, nil
}

func FindByID(d db.QueryReceiver, id int64) (*ProviderTemplate, error) {
	pt := &ProviderTemplate{}
	err := d.Get(pt, "SELECT * FROM provider_templates WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	return pt, nil
}

// FindByName returns the provider template with the specified name if found
func FindByName(d db.QueryReceiver, name string) (*ProviderTemplate, error) {
	pt := &ProviderTemplate{}
	if err := d.Get(pt, "SELECT * FROM provider_templates WHERE name = $1", name); err != nil {
		return nil, err
	}
	return pt, nil
}

func All(d db.QueryReceiver) ([]*ProviderTemplate, error) {
	pts := []*ProviderTemplate{}
	err := d.Select(&pts, "SELECT * FROM provider_templates ORDER BY id")
	if err != nil {
		return nil, err
	}
	return pts, nil
}

// Find is the recommended way to get a provider template that satisfies the
// provider, region and plan.
func Find(d db.QueryReceiver, region *region.Region, plan *plan.Plan, provider string) (*ProviderTemplate, error) {
	pt := &ProviderTemplate{}
	err := d.Get(pt, `SELECT * FROM provider_templates
		WHERE disabled = false AND
		region_id = $1 AND plan_id = $2 AND provider = $3`,
		region.ID, plan.ID, provider)
	if err != nil {
		return nil, err
	}
	return pt, nil
}

func (pt *ProviderTemplate) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO provider_templates
		(region_id, plan_id, name, provider, config, preprovision_max, disabled)
		VALUES (:region_id, :plan_id, :name, :provider, :config, :preprovision_max, :disabled)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pt).StructScan(pt)
}

func (pt *ProviderTemplate) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE provider_templates
		SET (name, provider, config, preprovision_max, disabled, updated_at)
		= (:name, :provider, :config, :preprovision_max, :disabled, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pt).StructScan(pt)
}
