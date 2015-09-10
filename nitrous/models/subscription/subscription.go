package subscription

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
)

// State can hold both our own State as well
// as well as state returned from Stripe
const (
	// subcription is in the process of being created
	StateCreating = "creating"
	// subscription has been cancelled
	StateDeleted = "deleted"
)

type Subscription struct {
	ID               int64  `db:"id"`
	PlanID           int64  `db:"plan_id"`
	BillingProfileID int64  `db:"billing_profile_id"`
	Slug             string `db:"slug"`

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

type SubscriptionWithHostSlug struct {
	*Subscription
	HostSlug sql.NullString `db:"host_slug"`
	OrgID    sql.NullInt64  `db:"org_id"`
}

func (s *Subscription) Insert(tx *sqlx.Tx) error {
	nstmt, err := tx.PrepareNamed(`INSERT INTO subscriptions
		(plan_id, billing_profile_id, state, stripe_id, slug)
    VALUES (:plan_id, :billing_profile_id, :state, :stripe_id, :slug)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRow(s).StructScan(s)
}

func (s *Subscription) Update(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`UPDATE subscriptions
		SET (state, plan_id, stripe_id, current_period_start, current_period_end, updated_at)
		= (:state, :plan_id, :stripe_id, :current_period_start, :current_period_end, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(s).StructScan(s)
}

func Find(d db.QueryReceiver, id int64) (*Subscription, error) {
	sub := &Subscription{}
	err := d.Get(sub, "SELECT * FROM subscriptions WHERE id = $1;", id)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func FindBySlug(d db.QueryReceiver, slug string, userID int64) (*Subscription, error) {
	sub := &Subscription{}
	err := d.Get(sub, `SELECT s.* FROM subscriptions s
		INNER JOIN billing_profiles ON billing_profiles.id = s.billing_profile_id
                LEFT JOIN hosts ON hosts.subscription_id = s.id
		WHERE s.slug = $1 AND
                (
                        (billing_profiles.user_id = $2) OR
                        (hosts.user_id = $2 AND billing_profiles.org_id IS NOT NULL) OR
                        (billing_profiles.org_id IS NOT NULL AND
                                billing_profiles.org_id IN (SELECT org_id FROM org_memberships WHERE user_id = $2 AND role = $3)
                        )
                );`, slug, userID, orgmembership.RoleOwner)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func FindByUserID(d db.QueryReceiver, id, userID int64) (*Subscription, error) {
	sub := &Subscription{}
	err := d.Get(sub, `SELECT subscriptions.* FROM subscriptions
		INNER JOIN billing_profiles ON billing_profiles.id = subscriptions.billing_profile_id
		WHERE subscriptions.id = $1 AND billing_profiles.user_id = $2;`,
		id, userID)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func FindAllActiveForOrg(d db.QueryReceiver, orgID int64) ([]*SubscriptionWithHostSlug, error) {
	subscriptions := []*SubscriptionWithHostSlug{}
	err := d.Select(&subscriptions, `SELECT s.*, hosts.slug AS host_slug, bp.org_id AS org_id FROM subscriptions s
                JOIN billing_profiles bp ON bp.id = s.billing_profile_id
                LEFT JOIN hosts ON hosts.subscription_id = s.id
                WHERE bp.org_id = $1 AND s.cancelled_at IS NULL
                ORDER BY s.created_at ASC;`, orgID)
	if err != nil {
		return nil, err
	}
	return subscriptions, nil
}

func FindAllActiveByUserID(d db.QueryReceiver, userID int64) ([]*SubscriptionWithHostSlug, error) {
	subscriptions := []*SubscriptionWithHostSlug{}
	err := d.Select(&subscriptions, `SELECT s.*, hosts.slug AS host_slug, bp.org_id AS org_id FROM subscriptions s
                JOIN billing_profiles bp ON bp.id = s.billing_profile_id
                LEFT JOIN hosts ON hosts.subscription_id = s.id
                WHERE (
                        (bp.user_id = $1 AND bp.org_id IS NULL) OR
                        (bp.org_id IS NOT NULL AND hosts.user_id = $1)
                ) AND s.cancelled_at IS NULL
                ORDER BY s.created_at ASC;`, userID)
	if err != nil {
		return nil, err
	}
	return subscriptions, nil
}

func (s *Subscription) CanBeCanceled() bool {
	return s.State == "canceled" || s.CancelPending
}
