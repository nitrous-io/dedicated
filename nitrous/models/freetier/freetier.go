package freetier

import (
	"errors"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

var (
	ErrFreePlanNotAllowed = errors.New("paying user is not allowed to create a free tier workspace")
	ErrPaidPlanNotAllowed = errors.New("free tier user is not allowed to create a paid workspace")
	ErrMaxFreeHostReached = errors.New("free tier user already has existing free host")
	ErrPhoneAuthRequired  = errors.New("phone authentication required")

	ErrLastRunningAtInvalid = errors.New("last_running_at is invalid")
)

// Checks whether a free/paid user can create a host of given plan.
func CreateHostWithPlanAllowed(d *db.Database, u *user.User, bp *billingprofile.BillingProfile, p *plan.Plan) error {
	if bp.IsFreeTier() && !bp.OrgID.Valid {
		if !p.IsFree() {
			// Free-tier user must upgrade their billing profile first before creating paid plan
			return ErrPaidPlanNotAllowed
		}

		if err := createFreeTierHostAllowed(d, u); err != nil {
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
