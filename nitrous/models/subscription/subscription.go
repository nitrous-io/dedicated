package subscription

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Subscription struct {
	ID         int64      `db:"id"`
	UserID     int64      `db:"user_id"`
	HostID     int64      `db:"host_id"`
	PlanID     int64      `db:"plan_id"`
	StripeID   string     `db:"stripe_id"`
	DisabledAt *time.Time `db:"disabled_at"`
	CreatedAt  *time.Time `db:"created_at"`
	UpdatedAt  *time.Time `db:"updated_at"`
}

func (s *Subscription) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO subscriptions
		(user_id, host_id, plan_id, stripe_id)
		VALUES (:user_id, :host_id, :plan_id, :stripe_id)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(s).StructScan(s)
}

func (s *Subscription) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE subscriptions
		SET (user_id, host_id, disabled_at, plan_id, stripe_id, updated_at)
		= (:user_id, :host_id, :disabled_at, :plan_id, :stripe_id, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(s).StructScan(s)
}
