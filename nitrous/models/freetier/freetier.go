package freetier

import (
	"errors"
	"strconv"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/region"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

var (
	ErrFreePlanNotAllowed = errors.New("paying user is not allowed to create a free tier workspace")
	ErrPaidPlanNotAllowed = errors.New("free tier user is not allowed to create a paid workspace")
	ErrMaxFreeHostReached = errors.New("free tier user already has existing free host")
	ErrPhoneAuthRequired  = errors.New("phone authentication required")

	ErrFreeTimeLimitExceeded      = errors.New("free tier user has exceeded daily usage quota")
	ErrFreeDailyHostLimitExceeded = errors.New("free tier workstation has exceeded daily quota in this region")
	ErrLastRunningAtInvalid       = errors.New("last_running_at is invalid")
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
		err := createFreeTierHostAllowed(d, u, nil, nil)
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

func CreateHostWithPlanAllowed(d *db.Database, u *user.User, bp *billingprofile.BillingProfile, p *plan.Plan, reg *region.Region) error {
	if bp.IsFreeTier() && !bp.OrgID.Valid {
		if !p.IsFree() {
			// Free-tier user must upgrade their billing profile first before creating paid plan
			return ErrPaidPlanNotAllowed
		}

		if err := createFreeTierHostAllowed(d, u, p, reg); err != nil {
			return err
		}
		return nil
	} else {
		// Organizations & Paid users cannot create hosts with free plan. Doesn't apply to admin
		// users so that we can test more easily
		if p.IsFree() && (bp.OrgID.Valid || !u.Admin) {
			return ErrFreePlanNotAllowed
		}
	}

	return nil
}

// Checks whether a user of given billing profile can create new free tier host.
// It returns ErrFreePlanNotAllowed if user is not in free tier or if free user has created a host
func createFreeTierHostAllowed(d *db.Database, u *user.User, p *plan.Plan, reg *region.Region) error {
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

	err = d.QueryRow(`
		SELECT count(*) FROM hosts
		WHERE region_id = $1
		AND plan_id = $2
		AND created_at > NOW() - '1 day'::INTERVAL
	`, reg.ID, p.ID).Scan(&count)
	if err != nil {
		return err
	}

	if p != nil && reg != nil {
		freeHostQuota, _ := strconv.Atoi(envvars.FreeHostQuota)
		if count > freeHostQuota {
			return ErrFreeDailyHostLimitExceeded
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
			AND (
				EXTRACT(EPOCH FROM (now() - last_pinged_at)) > idle_timeout
				-- if host started but IDE is never started, last_pinged_at will be null
				OR
				last_pinged_at IS NULL
			)
			-- if host has been running for less than 2700 seconds = 45 minutes; not idle
			AND 
			  (last_running_at IS NULL OR MOD(CAST(EXTRACT(EPOCH FROM now() - last_running_at) as numeric), 3600) > 2700)
			AND terminated_at IS NULL
		ORDER BY last_pinged_at ASC
	`, true, host.StateRunning)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func IsIdle(d *db.Database, h *host.Host) (bool, error) {
	var count int
	err := d.QueryRow(`
		SELECT count(*) FROM hosts
		WHERE id = $1
			AND auto_shutdown = $2
			AND state = $3
			AND (
				EXTRACT(EPOCH FROM (now() - last_pinged_at)) > idle_timeout
				-- if host started but IDE is never started, last_pinged_at will be null
				OR
				last_pinged_at IS NULL
			)
			-- if host has been running for less than 2700 seconds = 45 minutes; not idle
			AND 
			  (last_running_at IS NULL OR MOD(CAST(EXTRACT(EPOCH FROM now() - last_running_at) as numeric), 3600) > 2700)
			AND terminated_at IS NULL
		`, h.ID, true, host.StateRunning).Scan(&count)
	if err != nil {
		return false, err
	}

	return (count == 1), nil
}
