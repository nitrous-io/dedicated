package freetier

import (
	"errors"
	"time"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/plan"
)

var (
	ErrFreePlanNotAllowed    = errors.New("You are not allowed to create a free tier workspace")
	ErrPaidPlanNotAllowed    = errors.New("You are not allowed to create a paid workspace")
	ErrMaxFreeHostReached    = errors.New("You can't create any more free workspaces.")
	ErrFreeTimeLimitExceeded = errors.New("you have exceeded the free time limit allowed")
)

type HostUsage struct {
	HostID    int64       `db:"host_id"`
	Start     time.Time   `db:"created_at"`
	End       pq.NullTime `db:"ended_at"`
	UserID    int64       `db:"user_id"`
	HostState string      `db:"host_state"`
	MaxHours  int64       `db:"max_hours"`
}

type UserUsage struct {
	UserID int64

	// Maximum hours the free tier can use per month
	MaxHours int64

	// Total usage time by user in minutes
	UsageTime int64

	// Set of running hosts implemented as map
	RunningHosts map[int64]bool
}

func AllFreeTierHosts(d *db.Database, start, end time.Time) ([]*HostUsage, error) {
	query := `SELECT 
		host_transitions.host_id, host_transitions.created_at, 
		host_transitions.ended_at, hosts.state AS host_state,
		users.id AS user_id, billing_profiles.max_hours
		FROM host_transitions
			INNER JOIN hosts ON (hosts.id = host_transitions.host_id)
			INNER JOIN users ON (users.id = hosts.user_id)
			INNER JOIN billing_profiles ON (billing_profiles.user_id = users.id)
		WHERE
			host_transitions.state='running'
			AND host_transitions.created_at < $2
			AND COALESCE(host_transitions.ended_at, now()) > $1
			AND billing_profiles.state = 'free';`

	res := []*HostUsage{}
	err := d.Select(&res, query, start)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func FilterTimeExceedHosts(usages []*HostUsage, start, end time.Time) []int64 {
	userMap := map[int64]*UserUsage{}

	for _, usage := range usages {
		userID := usage.UserID

		userUsage := userMap[userID]
		if userUsage == nil {
			userUsage = &UserUsage{
				UserID:   userID,
				MaxHours: usage.MaxHours,
			}
			userMap[userID] = userUsage
		}

		usageStart := usage.Start
		if usageStart.Before(start) {
			usageStart = start
		}

		usageEnd := usage.End.Time
		if !usage.End.Valid {
			usageEnd = end
		}

		if usageEnd.After(end) {
			usageEnd = end
		}

		usageTime := int64(usageEnd.Sub(usageStart).Minutes())
		userUsage.UsageTime += usageTime

		if usage.HostState == "running" {
			userUsage.RunningHosts[usage.HostID] = true
		}
	}

	filteredHosts := []int64{}

	for _, userUsage := range userMap {
		if userUsage.UsageTime > (userUsage.MaxHours * 60) {
			for hostID, _ := range userUsage.RunningHosts {
				filteredHosts = append(filteredHosts, hostID)
			}
		}
	}

	return filteredHosts
}

func FindTimeExceededHosts(d *db.Database, start, end time.Time) ([]int64, error) {
	hostUsages, err := AllFreeTierHosts(d, start, end)
	if err != nil {
		return nil, err
	}

	return FilterTimeExceedHosts(hostUsages, start, end), nil
}

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
