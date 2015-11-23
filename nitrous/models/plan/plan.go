package plan

import (
	"database/sql"
	"math"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

const (
	TypeUsageBased = "usage-based"
	TypePrepaid    = "prepaid"
)

type Plan struct {
	ID                int64         `db:"id"`
	BasePlanID        sql.NullInt64 `db:"base_plan_id"`
	PaidPlanID        sql.NullInt64 `db:"paid_plan_id"`
	HostingModePlanID sql.NullInt64 `db:"hosting_mode_plan_id"`

	Name         string         `db:"name"`
	Description  string         `db:"description"`
	Slug         string         `db:"slug"`
	Type         string         `db:"type"`
	Price        int64          `db:"price"`
	BasePrice    int64          `db:"base_price"`
	CPU          string         `db:"cpu"`
	Memory       string         `db:"memory"`
	Storage      string         `db:"storage"`
	MostPopular  bool           `db:"most_popular"`
	StripePlanID sql.NullString `db:"stripe_plan_id"`
	CreatedAt    *time.Time     `db:"created_at"`
	UpdatedAt    *time.Time     `db:"updated_at"`

	// Disabled plans cannot be used and are left in the db for historical or
	// foreign key referential integrity purposes.
	Disabled bool `db:"disabled"`

	// Hidden plans are not displayed (but can be used, unless disabled).
	Hidden bool `db:"hidden"`

	TrialPeriodDays int64 `db:"trial_period_days"`

	// Features of a plan below. As a rule, any boolean features should default
	// to false (i.e. disabled), so choose names accordingly.

	AutoShutdown    bool  `db:"auto_shutdown"`
	MaxContainers   int64 `db:"max_containers"`
	MaxSnapshots    int64 `db:"max_snapshots"`
	CleanPreview    bool  `db:"clean_preview"`
	DesktopApp      bool  `db:"desktop_app"`
	SSH             bool  `db:"ssh"`
	CustomDomain    bool  `db:"custom_domain"`
	CustomTemplates bool  `db:"custom_templates"`
	Org             bool  `db:"org"`

	MaxHosts sql.NullInt64 `db:"max_hosts"`
	// MaxCollaborators has default value of 1. Where org=true, default is NULL.
	MaxCollaborators sql.NullInt64 `db:"max_collaborators"`
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

func (p *Plan) CanUpgradeToPlan(newPlan *Plan) bool {
	if p == nil {
		return false
	}
	if newPlan == nil {
		return false
	}
	if p.Org != newPlan.Org {
		return false
	}
	if p.MaxHosts.Valid != newPlan.MaxHosts.Valid {
		return false
	}

	return newPlan.Price > p.Price
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

func FindByStripePlanID(d db.QueryReceiver, slug string) (*Plan, error) {
	plan := &Plan{}
	if err := d.Get(plan, `SELECT * FROM plans WHERE stripe_plan_id = $1 AND disabled = $2`, slug, false); err != nil {
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
	if err := d.Select(&pHs, `SELECT
		hosts.id AS host_id, plans.price AS plan_price, plans.base_price AS base_plan_price
		FROM hosts INNER JOIN plans ON hosts.plan_id = plans.id
		WHERE hosts.user_id = $1 AND hosts.subscription_id IS NULL;`, userID); err != nil {
		return nil, err
	}

	mapping := make(map[int64]PlanPricing)
	for _, pH := range pHs {
		mapping[pH.HostID] = PlanPricing{Price: pH.PlanPrice, BasePrice: pH.PlanBasePrice}
	}
	return mapping, nil
}

func PlanPricingHostMapWithSubscription(d db.QueryReceiver, userID int64) (map[int64]PlanPricing, error) {
	type planHost struct {
		HostID        int64 `db:"host_id"`
		PlanPrice     int64 `db:"plan_price"`
		PlanBasePrice int64 `db:"base_plan_price"`
	}
	pHs := []*planHost{}
	if err := d.Select(&pHs, `SELECT
		hosts.id AS host_id, plans.price AS plan_price, plans.base_price AS base_plan_price
		FROM hosts INNER JOIN plans ON hosts.plan_id = plans.id
		WHERE hosts.user_id = $1;`, userID); err != nil {
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
		) AND (
			plans.price > 0
			OR plans.base_price > 0
		)
		ORDER BY (price, max_containers, name);
	`, provider)

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
		AND NOT hidden
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

func PlansForMultiHostOrgs(d db.QueryReceiver, orgIds []int64) (map[int64]*Plan, error) {
	mp := make(map[int64]*Plan)
	if len(orgIds) == 0 {
		return mp, nil
	}

	type orgPlan struct {
		*Plan
		OrgID int64 `db:"org_id"`
	}
	plans := []*orgPlan{}
	q, args, err := sqlx.In(`SELECT plans.*, bp.org_id FROM plans
		INNER JOIN subscriptions s ON s.plan_id = plans.id
		INNER JOIN billing_profiles bp ON s.billing_profile_id = bp.id
		WHERE s.multi_host IS TRUE
		AND s.cancelled_at IS NULL
		AND bp.org_id IN (?)`, orgIds)
	if err != nil {
		return mp, err
	}
	if err := d.Select(&plans, d.Rebind(q), args...); err != nil {
		return mp, err
	}

	for _, pl := range plans {
		mp[pl.OrgID] = pl.Plan
	}

	return mp, nil
}

func (p *Plan) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO plans
		(slug, name, max_containers, price, base_price, cpu, memory, storage, disabled, hidden, most_popular, base_plan_id, paid_plan_id, stripe_plan_id, auto_shutdown, org, max_hosts, max_collaborators)
		VALUES (:slug, :name, :max_containers, :price, :base_price, :cpu, :memory, :storage, :disabled, :hidden, :most_popular, :base_plan_id, :paid_plan_id, :stripe_plan_id, :auto_shutdown, :org, :max_hosts, :max_collaborators)
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

// TrialEnd returns the Unix time of when a trial should end, given startTime
// as Unix time.  Returns 0 if the plan does not support free trials
// (trial_period_days = 0)
func (p *Plan) TrialEnd(startTime int64) int64 {
	if p.TrialPeriodDays == 0 {
		return 0
	}
	return (time.Unix(startTime, 0).Add(time.Duration(p.TrialPeriodDays*24) * time.Hour)).Unix()
}
