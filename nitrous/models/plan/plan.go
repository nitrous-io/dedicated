package plan

import (
	"math"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Plan struct {
	ID            int64      `db:"id"`
	Name          string     `db:"name"`
	Slug          string     `db:"slug"`
	MaxContainers int64      `db:"max_containers"`
	Price         int64      `db:"price"`
	CPU           string     `db:"cpu"`
	Memory        string     `db:"memory"`
	Storage       string     `db:"storage"`
	Disabled      bool       `db:"disabled"`
	MostPopular   bool       `db:"most_popular"`
	CreatedAt     *time.Time `db:"created_at"`
	UpdatedAt     *time.Time `db:"updated_at"`
}

func FindByID(d db.QueryReceiver, id int64) (*Plan, error) {
	plan := &Plan{}
	err := d.Get(plan, "SELECT * FROM plans WHERE id = $1", id)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func FindBySlug(d db.QueryReceiver, slug string) (*Plan, error) {
	plan := &Plan{}
	err := d.Get(plan, `SELECT * FROM plans WHERE slug = $1 AND disabled = $2`, slug, false)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func All(d db.QueryReceiver) ([]*Plan, error) {
	pls := []*Plan{}
	if err := d.Select(&pls, `SELECT * FROM plans ORDER BY (price, max_containers, name)`); err != nil {
		return nil, err
	}
	return pls, nil
}

func Enabled(d db.QueryReceiver) ([]*Plan, error) {
	pls := []*Plan{}
	if err := d.Select(&pls, `SELECT * FROM plans WHERE disabled = false ORDER BY (price, max_containers, name);`); err != nil {
		return nil, err
	}
	return pls, nil
}

func PlanPricingHostMap(d db.QueryReceiver, userID int64) (map[int64]int64, error) {
	type planHost struct {
		HostID    int64 `db:"host_id"`
		PlanPrice int64 `db:"plan_price"`
	}
	pHs := []*planHost{}
	if err := d.Select(&pHs, `SELECT hosts.id AS host_id, plans.price AS plan_price FROM hosts INNER JOIN plans ON hosts.plan_id = plans.id WHERE hosts.user_id = $1`, userID); err != nil {
		return nil, err
	}

	mapping := make(map[int64]int64)
	for _, pH := range pHs {
		mapping[pH.HostID] = pH.PlanPrice
	}
	return mapping, nil
}

func (p *Plan) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO plans
		(slug, name, max_containers, price, cpu, memory, storage, disabled, most_popular)
		VALUES (:slug, :name, :max_containers, :price, :cpu, :memory, :storage, :disabled, :most_popular)
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
