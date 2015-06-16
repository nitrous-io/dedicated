package plan

import (
	"database/sql"
	"math"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Plan struct {
	ID            int64          `db:"id"`
	Name          string         `db:"name"`
	Slug          string         `db:"slug"`
	MaxContainers int64          `db:"max_containers"`
	Price         int64          `db:"price"`
	BasePrice     int64          `db:"base_price"`
	CPU           string         `db:"cpu"`
	Memory        string         `db:"memory"`
	Storage       string         `db:"storage"`
	Disabled      bool           `db:"disabled"`
	MostPopular   bool           `db:"most_popular"`
	PaidPlanID    sql.NullInt64  `db:"paid_plan_id"`
	StripePlanID  sql.NullString `db:"stripe_plan_id"`
	CreatedAt     *time.Time     `db:"created_at"`
	UpdatedAt     *time.Time     `db:"updated_at"`
}

type PlanPricing struct {
	Price     int64 `db:"price"`
	BasePrice int64 `db:"base_price"`
}

func (p *Plan) IsFree() bool {
	return p.Price == 0 && p.BasePrice == 0
}

func (p *Plan) IsSubscription() bool {
	return p.StripePlanID.Valid
}

func FindByID(d db.QueryReceiver, id int64) (*Plan, error) {
	plan := &Plan{}
	if err := d.Get(plan, "SELECT * FROM plans WHERE id = $1", id); err != nil {
		return nil, err
	}
	return plan, nil
}

func FindBySlug(d db.QueryReceiver, slug string) (*Plan, error) {
	plan := &Plan{}
	if err := d.Get(plan, `SELECT * FROM plans WHERE slug = $1 AND disabled = $2`, slug, false); err != nil {
		return nil, err
	}
	return plan, nil
}

func PlanPricingHostMap(d db.QueryReceiver, userID int64) (map[int64]PlanPricing, error) {
	type planHost struct {
		HostID        int64 `db:"host_id"`
		PlanPrice     int64 `db:"plan_price"`
		PlanBasePrice int64 `db:"base_plan_price"`
	}
	pHs := []*planHost{}
	if err := d.Select(&pHs, `SELECT hosts.id AS host_id, plans.price AS plan_price, plans.base_price AS base_plan_price FROM hosts INNER JOIN plans ON hosts.plan_id = plans.id WHERE hosts.user_id = $1`, userID); err != nil {
		return nil, err
	}

	mapping := make(map[int64]PlanPricing)
	for _, pH := range pHs {
		mapping[pH.HostID] = PlanPricing{Price: pH.PlanPrice, BasePrice: pH.PlanBasePrice}
	}
	return mapping, nil
}

func PaidPlansForProvider(d db.QueryReceiver, provider string) ([]*Plan, error) {
	plans := []*Plan{}
	err := d.Select(&plans, `
		SELECT * FROM plans
		WHERE NOT disabled
		AND plans.id IN (
			SELECT DISTINCT plan_id
			FROM provider_templates AS pt
			WHERE pt.provider=$1 AND NOT pt.disabled
		)
		AND (
			plans.price > 0
			OR plans.base_price > 0
		)
		ORDER BY (price, max_containers, name);`, provider)

	if err != nil {
		return nil, err
	}
	return plans, nil
}

func AllPlansForProvider(d db.QueryReceiver, provider string) ([]*Plan, error) {
	plans := []*Plan{}
	err := d.Select(&plans, `
		SELECT * FROM plans
		WHERE NOT disabled
		AND plans.id IN (
			SELECT DISTINCT plan_id
			FROM provider_templates AS pt
			WHERE pt.provider=$1 AND NOT pt.disabled
		)
		ORDER BY (price, max_containers, name);`, provider)

	if err != nil {
		return nil, err
	}
	return plans, nil
}

func (p *Plan) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO plans
		(slug, name, max_containers, price, base_price, cpu, memory, storage, disabled, most_popular, paid_plan_id, stripe_plan_id)
		VALUES (:slug, :name, :max_containers, :price, :base_price, :cpu, :memory, :storage, :disabled, :most_popular, :paid_plan_id, :stripe_plan_id)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(p).StructScan(p)
}

// 672 = 28 * 24 (hourly price is derived by using monthly
// price for 28 days)
// hourly price is millicents, so *1000
func (p *Plan) HourlyPrice() int64 {
	return int64(math.Ceil(float64(p.Price) / 672 * 1000))
}
