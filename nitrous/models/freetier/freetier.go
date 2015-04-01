package freetier

import (
	"net/http"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errFreePlanNotAllowed = httperror.New(http.StatusForbidden, "free_plan_not_allowed",
		"You are not allowed to create a free tier workspace")

	errPaidPlanNotAllowed = httperror.New(http.StatusForbidden, "paid_plan_not_allowed",
		"You are not allowed to create a paid workspace")

	errMaxFreeHostReached = httperror.New(http.StatusForbidden, "max_free_host_reached",
		"You can't create any more free workspaces.")

	errFreeTimeLimitExceeded = httperror.New(http.StatusForbidden, "free_time_limit_exceeded",
		"you have exceeded the free time limit allowed")
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
			return errPaidPlanNotAllowed
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
			return errMaxFreeHostReached
		}

		if usage >= bp.MaxMinutes() {
			return errFreeTimeLimitExceeded
		}
	} else {
		// Paid users cannot create hosts with free plan
		if p.IsFree() {
			return errFreePlanNotAllowed
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
		return errFreeTimeLimitExceeded
	}

	return nil
}
