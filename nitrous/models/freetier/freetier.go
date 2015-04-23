package freetier

import (
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/freequeue"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

var (
	ErrFreePlanNotAllowed = errors.New("paying user is not allowed to create a free tier workspace")
	ErrPaidPlanNotAllowed = errors.New("free tier user is not allowed to create a paid workspace")
	ErrMaxFreeHostReached = errors.New("free tier user already has existing free host")
	ErrPhoneAuthRequired  = errors.New("phone authentication required")

	ErrFreeTimeLimitExceeded = errors.New("free tier user has exceeded daily usage quota")
	ErrLastRunningAtInvalid  = errors.New("last_running_at is invalid")
	ErrUpgradeNotAvailable   = errors.New("host has no upgradable paid plan")
)

func upgradeHostToPaid(d *db.Database, h *host.Host, currentPlan *plan.Plan) error {
	if !currentPlan.PaidPlanID.Valid {
		return ErrUpgradeNotAvailable
	}

	paidPlan, err := plan.FindByID(d, currentPlan.PaidPlanID.Int64)
	if err != nil {
		return err
	}

	h.PlanID = paidPlan.ID
	h.Size = sql.NullString{Valid: true, String: paidPlan.Slug}
	h.HourlyPrice = paidPlan.HourlyPrice()
	h.MaxContainers = sql.NullInt64{Valid: true, Int64: paidPlan.MaxContainers}
	h.LastRunningAt = pq.NullTime{Valid: false}

	nstmt, err := d.PrepareNamed(`UPDATE hosts
		SET (plan_id, size, hourly_price, max_containers, last_running_at)
		= (:plan_id, :size, :hourly_price, max_containers, :last_running_at)
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}

	err = nstmt.QueryRowx(h).StructScan(h)
	if err != nil {
		return err
	}

	return nil
}

// Upgrade all hosts to paid for free-tier user
// just upgraded to paid tier
func UpgradeFreeUserHosts(d *db.Database, u *user.User) error {
	hosts, err := host.FindAllByUser(d, u)
	if err != nil {
		return err
	}

	// Free tier user should only have one host
	// after upgrading, but putting this in a loop anyway
	for _, h := range hosts {
		if h.HourlyPrice == 0 {
			p, err := plan.FindByID(d, h.PlanID)
			if err != nil {
				return err
			}

			err = upgradeHostToPaid(d, h, p)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func EligiblePlansForUser(d *db.Database, u *user.User, provider string) ([]*plan.Plan, error) {
	bp, err := u.BillingProfile(d)
	if err != nil {
		return nil, err
	}

	if bp.IsFreeTier() {
		err := CreateFreeTierHostAllowed(d, u)
		switch err {
		case nil, ErrPhoneAuthRequired:
			return plan.AllPlansForProvider(d, provider)
		case ErrFreePlanNotAllowed, ErrMaxFreeHostReached:
			return plan.PaidPlansForProvider(d, provider)
		default:
			return nil, err
		}

	} else {
		return plan.PaidPlansForProvider(d, provider)
	}
}

// Checks whether a user of given billing profile can create new free tier host.
// It returns ErrFreePlanNotAllowed if user is not in free tier or if free user has created a host
func CreateFreeTierHostAllowed(d *db.Database, u *user.User) error {
	bp, err := u.BillingProfile(d)
	if err != nil {
		return err
	}

	if !bp.IsFreeTier() {
		return ErrFreePlanNotAllowed
	}

	if !u.PhoneAuthed {
		return ErrPhoneAuthRequired
	}

	hosts, err := host.FindAllWithTerminated(d, bp.UserID)
	if err != nil {
		return err
	}

	if len(hosts) > 0 {
		return ErrMaxFreeHostReached
	}

	return nil
}

// Checks whether a free/paid user can create a host of given plan.
func CreateHostWithPlanAllowed(d *db.Database, u *user.User, p *plan.Plan) error {
	bp, err := u.BillingProfile(d)
	if err != nil {
		return err
	}

	if bp.IsFreeTier() {
		if !p.IsFree() {
			// Free-tier user must upgrade their billing profile first before creating paid plan
			return ErrPaidPlanNotAllowed
		}

		if err := CreateFreeTierHostAllowed(d, u); err != nil {
			return err
		}

		return freequeue.UserApproved(d, u)

	} else {
		// Paid users cannot create hosts with free plan. Doesn't apply to admin
		// users so that we can test more easily
		if p.IsFree() && !u.Admin {
			return ErrFreePlanNotAllowed
		}
	}

	return nil
}

func StopHostAllowed(d *db.Database, u *user.User) (bool, error) {
	bp, err := u.BillingProfile(d)
	if err != nil {
		return false, err
	}

	return !bp.IsFreeTier(), nil
}

func StartHostAllowed(d *db.Database, u *user.User, h *host.Host) error {
	bp, err := u.BillingProfile(d)
	if err != nil {
		return err
	}

	if !bp.IsFreeTier() {
		return nil
	}

	if !u.MaxRunningTime.Valid || !u.RefreshPeriod.Valid {
		return nil
	}

	if !h.LastRunningAt.Valid {
		return ErrLastRunningAtInvalid
	}

	lastTime := h.LastRunningAt.Time

	now := time.Now().UTC()

	// if the host was last running outside of the refresh period then the host can be started again
	// regardless of the quota they have used (because the quota has been reset because we are in a new period)
	refreshPeriod := lastTime.Add(time.Duration(u.RefreshPeriod.Int64) * time.Second)
	if refreshPeriod.Before(now) {
		return nil
	}

	used := int64(now.Sub(lastTime) / time.Second)
	if used >= u.MaxRunningTime.Int64 {
		return ErrFreeTimeLimitExceeded
	}

	return nil
}

func ExpiredHosts(d *db.Database) ([]*host.Host, error) {
	// query for hosts where their owners have limits and the hosts have exceeded these limits
	query := `SELECT hosts.*, pt.provider, pt.config
    FROM hosts
    INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
    INNER JOIN users ON hosts.user_id = users.id
    INNER JOIN billing_profiles bp ON bp.user_id = users.id
    WHERE
    	last_running_at IS NOT NULL
    	AND terminated_at IS NULL
    	AND preprovisioned IS FALSE
    	AND bp.state = $1
    	AND hosts.state = $2
    	AND users.max_running_time IS NOT NULL
    	AND users.refresh_period IS NOT NULL
    	AND (now() - hosts.last_running_at) > (interval '1 second' * users.max_running_time);`

	hosts := []*host.Host{}
	err := d.Select(&hosts, query, billingprofile.StateFree, host.StateRunning)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}
