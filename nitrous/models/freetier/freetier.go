package freetier

import (
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
)

var (
	ErrFreePlanNotAllowed    = errors.New("paying user is not allowed to create a free tier workspace")
	ErrPaidPlanNotAllowed    = errors.New("free tier user is not allowed to create a paid workspace")
	ErrMaxFreeHostReached    = errors.New("free tier user already has existing free host")
	ErrFreeTimeLimitExceeded = errors.New("free tier user has exceeded monthly usage quota")
)

func EligiblePlansForUser(d *db.Database, u *user.User, provider string) ([]*plan.Plan, error) {
	bp, err := u.BillingProfile(d)
	if err != nil {
		return nil, err
	}

	if bp.IsFreeTier() {
		err := CreateFreeTierHostAllowed(d, bp)
		switch err {
		case nil:
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
func CreateFreeTierHostAllowed(d *db.Database, bp *billingprofile.BillingProfile) error {
	if !bp.IsFreeTier() {
		return ErrFreePlanNotAllowed
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
func CreateHostWithPlanAllowed(d *db.Database, bp *billingprofile.BillingProfile, p *plan.Plan) error {
	if bp.IsFreeTier() {
		if !p.IsFree() {
			// Free-tier user must upgrade their billing profile first before creating paid plan
			return ErrPaidPlanNotAllowed
		}

		return CreateFreeTierHostAllowed(d, bp)

	} else {
		// Paid users cannot create hosts with free plan
		if p.IsFree() {
			return ErrFreePlanNotAllowed
		}
	}

	return nil
}

func StartHostAllowed(d *db.Database, bp *billingprofile.BillingProfile, now time.Time) error {
	if !bp.IsFreeTier() {
		return nil
	}

	usage, _, err := hosttransition.CurrentMonthUsageForUser(d, bp.UserID, now)
	if err != nil {
		return err
	}

	if usage >= bp.MaxMinutes() {
		return ErrFreeTimeLimitExceeded
	}

	return nil
}
