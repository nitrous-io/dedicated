package subscription

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// State holds both Stripe & Nitrous custom states
const (
	// subcription is in the process of being created
	StateCreating = "creating"
	// subscription has been cancelled
	StateDeleted = "deleted"
)

type Subscription struct {
	ID               int64         `db:"id"`
	PlanID           int64         `db:"plan_id"`
	BillingProfileID int64         `db:"billing_profile_id"`
	HostID           sql.NullInt64 `db:"host_id"`

	StripeID sql.NullString `db:"stripe_id"`
	State    string         `db:"state"`

	CurrentPeriodStart pq.NullTime `db:"current_period_start"`
	CurrentPeriodEnd   pq.NullTime `db:"current_period_end"`

	CancelPending bool        `db:"cancel_pending"`
	CancelledAt   pq.NullTime `db:"cancelled_at"`

	// to facilitate downgrades we need to cancel the current subscription
	// wait for it to notify us of a delete, then create a new one with the plan
	PendingPlanID sql.NullInt64 `db:"pending_plan_id"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func (s *Subscription) Insert(tx *sqlx.Tx) error {
	nstmt, err := tx.PrepareNamed(`INSERT INTO subscriptions
		(plan_id, billing_profile_id, state, host_id, stripe_id)
    VALUES (:plan_id, :billing_profile_id, :state, :host_id, :stripe_id)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRow(s).StructScan(s)
}
