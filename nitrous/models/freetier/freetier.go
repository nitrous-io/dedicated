package freetier

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
)

var (
	errPaidPlanNotAllowed = httperror.New(http.StatusForbidden, "paid_plan_not_allowed",
		"free tier user is not allowed to create paid host plan")

	errMaxFreeHostReached = httperror.New(http.StatusForbidden, "max_free_host_reached",
		"you have reach the maximum number of free host allowed")

	errFreeTimeLimitExceeded = httperror.New(http.StatusForbidden, "free_time_limit_exceeded",
		"you have exceeded the free time limit allowed")
)

func HasNonTerminatedHost(hs []*host.Host) bool {
	for _, h := range hs {
		if h.TerminatedAt != nil {
			return true
		}
	}
	return false
}

func CreateHostPermission(d *db.Database, bp *billingprofile.BillingProfile, p *plan.Plan) error {
	// Free tier users cannot create hosts with paid plan
	if p.Price != 0 {
		return errPaidPlanNotAllowed
	}

	hs, err := host.FindAllWithTerminated(d, bp.UserID)
	if err != nil {
		return err
	}

	if HasNonTerminatedHost(hs) {
		return errMaxFreeHostReached
	}

	usage, err := hosttransition.CurrentMonthUsageForHosts(d, host.HostsToHostIDs(hs))
	if err != nil {
		return err
	}

	if usage >= bp.MaxMinutes() {
		return errFreeTimeLimitExceeded
	}

	return nil
}

func StartHostPermission(d *db.Database, bp *billingprofile.BillingProfile) error {
	usage, err := hosttransition.CurrentMonthUsageForUser(d, bp.UserID)
	if err != nil {
		return err
	}

	if usage >= bp.MaxMinutes() {
		return errFreeTimeLimitExceeded
	}

	return nil
}
