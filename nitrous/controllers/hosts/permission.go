package hosts

import (
	"net/http"
	"time"

	"github.com/jinzhu/now"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
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

func CheckCreateHostPermission(d *db.Database, u *user.User, p *plan.Plan) error {
	bp, err := billingprofile.Find(d, u.ID)
	if err != nil {
		return err
	}

	// Pass all non-free tier users for now
	if bp.State != billingprofile.StateFree {
		return nil
	}

	// Free tier users cannot create hosts with paid plan
	if p.Price != 0 {
		return errPaidPlanNotAllowed
	}

	hs, err := host.FindAllWithTerminated(d, u)
	if err != nil {
		return err
	}

	hostCount := len(hs)

	// Allow if user have not created any host
	if hostCount == 0 {
		return nil
	}

	hostIDs := make([]int64, hostCount)

	for i, h := range hs {
		hostIDs[i] = h.ID

		// Error if any of the host is active (non-terminated)
		// because only one max host is allowed
		if h.TerminatedAt != nil {
			return errMaxFreeHostReached
		}
	}

	start := now.BeginningOfMonth()
	end := time.Now()

	// All of the user's hosts have been terminated
	// Checking if the total time usage is more than
	// the time limit.
	usage, err := hosttransition.TotalUsageForHosts(d, hostIDs, start, end)
	if err != nil {
		return err
	}

	maxMinutes := bp.MaxHours.Int64 * 60

	if usage >= maxMinutes {
		return errFreeTimeLimitExceeded
	}

	return nil
}
