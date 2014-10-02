package providertemplate

import (
	"database/sql"
	"time"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type ProviderTemplate struct {
	ID        int64         `db:"id"`
	RegionID  int64         `db:"region_id"`
	PlanID    int64         `db:"plan_id"`
	Name      string        `db:"name"`
	Provider  string        `db:"provider"`
	Config    hstore.Hstore `db:"config"`
	CreatedAt *time.Time    `db:"created_at"`
	UpdatedAt *time.Time    `db:"updated_at"`
}

func FindByID(d db.QueryReceiver, id int64) (*ProviderTemplate, error) {
	pt := &ProviderTemplate{}
	err := d.Get(pt, "SELECT * FROM provider_templates WHERE id = $1", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return pt, nil
}

func FindByRegionAndPlanAndProvider(d db.QueryReceiver, regionID, planID int64, provider string) (*ProviderTemplate, error) {
	pt := &ProviderTemplate{}
	err := d.Get(pt, "SELECT * FROM provider_templates WHERE region_id = $1 AND plan_id = $2 AND provider = $3", regionID, planID, provider)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return pt, nil
}

func (pt *ProviderTemplate) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO provider_templates
		(region_id, plan_id, name, provider, config)
		VALUES (:region_id, :plan_id, :name, :provider, :config)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(pt).StructScan(pt)
}
