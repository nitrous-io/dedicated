package providertemplate

import (
	"time"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type ProviderTemplate struct {
	ID              int64         `db:"id"`
	RegionID        int64         `db:"region_id"`
	PlanID          int64         `db:"plan_id"`
	Name            string        `db:"name"`
	Provider        string        `db:"provider"`
	Config          hstore.Hstore `db:"config"`
	PreprovisionMax int           `db:"preprovision_max"`
	CreatedAt       *time.Time    `db:"created_at"`
	UpdatedAt       *time.Time    `db:"updated_at"`
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

func All(d db.QueryReceiver) ([]*ProviderTemplate, error) {
	pts := []*ProviderTemplate{}
	err := d.Select(&pts, "SELECT * FROM provider_templates ORDER BY id")
	if err != nil {
		return nil, err
	}
	return pts, nil
}

func FindByRegionAndPlanAndProvider(d db.QueryReceiver, regionID, planID int64, provider string) (*ProviderTemplate, error) {
	pt := &ProviderTemplate{}
	err := d.Get(pt, "SELECT * FROM provider_templates WHERE region_id = $1 AND plan_id = $2 AND provider = $3", regionID, planID, provider)
	if err != nil {
		return nil, err
	}
	return pt, nil
}

func (pt *ProviderTemplate) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO provider_templates
		(region_id, plan_id, name, provider, config, preprovision_max)
		VALUES (:region_id, :plan_id, :name, :provider, :config, :preprovision_max)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pt).StructScan(pt)
}

func (pt *ProviderTemplate) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE provider_templates
		SET (name, provider, config, preprovision_max, updated_at)
		= (:name, :provider, :config, :preprovision_max, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pt).StructScan(pt)
}
