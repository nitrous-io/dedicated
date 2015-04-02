package freetier

import (
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
)

var (
	ErrFreePlanNotAllowed    = errors.New("paying user is not allowed to create a free tier workspace")
	ErrPaidPlanNotAllowed    = errors.New("free tier user is not allowed to create a paid workspace")
	ErrMaxFreeHostReached    = errors.New("free tier user already has existing free host")
	ErrFreeTimeLimitExceeded = errors.New("free tier user has exceeded monthly usage quota")
)

func hasNonTerminatedHost(hs []*host.Host) bool {
	for _, h := range hs {
		if h.TerminatedAt == nil {
			return true
		}
	}
	return false
}

func CreateHostAllowed(d *db.Database, bp *billingprofile.BillingProfile, p *plan.Plan, now time.Time) error {
	if bp.IsFreeTier() {
		// Free tier users cannot create hosts with paid plan
		if p.Price != 0 {
			return ErrPaidPlanNotAllowed
		}

		usage, hosts, err := hosttransition.CurrentMonthUsageForUser(d, bp.UserID, now)
		if err != nil {
			return err
		}

		// Skip if user don't have any host created
		if len(hosts) == 0 {
			return nil
		}

		if hasNonTerminatedHost(hosts) {
			return ErrMaxFreeHostReached
		}

		if usage >= bp.MaxMinutes() {
			return ErrFreeTimeLimitExceeded
		}
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
