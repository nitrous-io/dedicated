package freetier

import (
	"errors"

	"github.com/nitrous-io/dedicated/nitrous/db"
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
)

func EligiblePlansForUser(d *db.Database, u *user.User, provider string) ([]*plan.Plan, error) {
	bp, err := u.BillingProfile(d)
	if err != nil {
		return nil, err
	}

	if bp.IsCardRequired() && !u.PhoneAuthed {
		return plan.AllPlansForProvider(d, provider)
	}

	if bp.IsFreeTier() {
		err := createFreeTierHostAllowed(d, u)
		switch err {
		case nil, ErrPhoneAuthRequired:
			return plan.AllPlansForProvider(d, provider)
		case ErrFreePlanNotAllowed, ErrMaxFreeHostReached:
			return plan.PaidPlansForProvider(d, provider)
		default:
			return nil, err
		}
	}

	return plan.PaidPlansForProvider(d, provider)
}

// Checks whether a user of given billing profile can create new free tier host.
// It returns ErrFreePlanNotAllowed if user is not in free tier or if free user has created a host
func createFreeTierHostAllowed(d *db.Database, u *user.User) error {
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

	// Exclude orphaned hosts so free users who have issues creating can create
	// another.
	var count int
	err = d.QueryRow(`
		SELECT count(*) FROM hosts
		WHERE user_id = $1
		AND state != $2
		AND preprovisioned = $3
		AND terminated_at IS NULL;
	`, u.ID, host.StateOrphan, false).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
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

		if err := createFreeTierHostAllowed(d, u); err != nil {
			return err
		}
		return nil
	} else {
		// Paid users cannot create hosts with free plan. Doesn't apply to admin
		// users so that we can test more easily
		if p.IsFree() && !u.Admin {
			return ErrFreePlanNotAllowed
		}
	}

	return nil
}

func IdleFreeHosts(d *db.Database) ([]*host.Host, error) {
	hosts := []*host.Host{}
	err := d.Select(&hosts, `SELECT hosts.*, pts.provider, pts.config
		FROM hosts
		INNER JOIN provider_templates AS pts on hosts.provider_template_id = pts.id
		WHERE auto_shutdown = $1
			AND state = $2
			AND EXTRACT(EPOCH FROM (now() - last_pinged_at)) > idle_timeout
			AND terminated_at IS NULL
		ORDER BY last_pinged_at ASC
	`, true, host.StateRunning)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}
