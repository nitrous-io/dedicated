package hostscan

import (
	"database/sql"
	"errors"
	"github.com/lib/pq"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
)

const (
	// Host is used more times than minimum usage
	StateActive = "active"

	// Host is currently in grace period
	StateGracePeriod = "grace"

	// Host owner is sent one week before termination notice
	StateOneWeekNotice = "one_week_notice"

	// Host is in terminating notice period
	StateTerminating = "terminating"

	// Host is terminated
	StateTerminated = "terminated"
)

const (
	// Host need to be started at least 4 times
	MinUsage = 4

	oneDay     time.Duration = 24 * time.Hour
	oneWeek    time.Duration = 7 * oneDay
	threeWeeks time.Duration = 3 * oneWeek
	fourWeeks  time.Duration = 4 * oneWeek

	// TODO: Change these to be configurable in envvars
	// We calculate host usage in four weeks period
	HostScanPeriod = fourWeeks

	// Warning one week before termination
	WarningPeriod = HostScanPeriod - oneWeek

	// Notify three days before terminating
	// Note that technically this is the period
	// considered when calculating minimum usage.
	TerminateNoticePeriod = HostScanPeriod - 3*oneDay
)

var (
	ErrHostIsNotFree    = errors.New("Scanned host is not in free plan")
	ErrUnknownScanState = errors.New("Unable to determine scan state of host")
	ErrNotYetNextScan   = errors.New("Host is still not need to be scanned")
)

/*
 * HostScan is a system to periodically scan for the usage of
 * a free host and determine which action to take, and when
 * should the next scan be performed.
 *
 * The actual scanning is scheduled by external system,
 * which will call hostscan functions to scan for hosts
 * with next scan time that has passed the current time.
 *
 * Since calling host scan is done externally, there is no
 * guarantee that a host will be scanned at the exact time
 * for its next scan. A job runner should call host scan
 * every hour or so to keep actions close to next scan time.
 *
 * Host scan also do _not_ take actual action on the hosts,
 * such as changing db, terminating hosts or sending announcements.
 *
 * Instead the scan result of a host is presented as a
 * HostScan struct, which the caller could then perform
 * actions based on the state field, and then save the
 * host scan result to db only after the action succeeded.
 */
type HostScan struct {
	ID             int64         `db:"id"`
	HostID         int64         `db:"host_id"`
	State          string        `db:"state"`
	AnnouncementID sql.NullInt64 `db:"announcement_id"`

	// The start of the period considered
	PeriodStart time.Time `db:"period_start"`

	// The end of the period considered
	PeriodEnd time.Time `db:"period_end"`

	// The time this scan is performed
	RecordedAt time.Time `db:"recorded_at"`

	// The time the next scan should be performed
	NextScan pq.NullTime `db:"next_scan"`

	// Indicates that the scan has been completed
	// and no subsequent scanning is needed.
	// This should be true if next_scan is NULL
	ScanCompleted bool `db:"scan_completed"`
}

// A scanned host is basically a host.Host with
// additional last scanned information.
// LastScanState will be non-empty string if
// host has been scanned before
type ScannedHost struct {
	*host.Host
	LastScanState sql.NullString `db:"last_scan_state"`

	// Recent usage of the host within the scan period
	// Filled in seperate sql query
	RecentUsage []*hosttransition.HostUsage
}

func (host *HostScan) Insert(d *db.Database) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO host_scans
		(host_id, state, announcement_id, period_start, period_end, recorded_at, next_scan, scan_completed)
		VALUES
		(:host_id, :state, :announcement_id, :period_start, :period_end, :recorded_at, :next_scan, :scan_completed)
		RETURNING *;
	`)
	if err != nil {
		return err
	}

	return nstmt.QueryRowx(host).StructScan(host)
}

// Get the start and end of the current period
// from a beginning time, current time, and the
// period duration. Returns the start/end of
// the period, and the time passed since start of the period.
func currentPeriod(begin, now time.Time, period time.Duration) (time.Time, time.Time, time.Duration) {
	diff := now.Sub(begin)
	timeSincePeriod := diff % period
	start := now.Add(-timeSincePeriod)
	end := start.Add(period)

	return start, end, timeSincePeriod
}

// Filter host transitions to include only the ones happening within given period.
func filterUsageByPeriod(usages []*hosttransition.HostUsage, start, end time.Time) []*hosttransition.HostUsage {
	filteredUsages := []*hosttransition.HostUsage{}

	for _, usage := range usages {
		// A usage is not in range if it ends either before start
		// or it starts after the end. Otherwise it is in range.
		if !(usage.Start.After(end) || usage.End.Before(start)) {
			filteredUsages = append(filteredUsages, usage)
		}
	}

	return filteredUsages
}

// Scan a host usage and determine what action to take.
// Accepts list of host transitions in the last 4 weeks
func ScanHost(host *ScannedHost, now time.Time) (*HostScan, error) {
	hostID := host.ID
	hostCreated := *host.CreatedAt

	// Sanity check that it is a free host
	if host.HourlyPrice > 0 {
		return nil, ErrHostIsNotFree
	}

	// Calculate the four-week period based on the host creation date.
	start, end, timePassed := currentPeriod(hostCreated, now, fourWeeks)

	// If the last scan state was terminating, terminate the host
	if host.LastScanState.Valid && host.LastScanState.String == StateTerminating {
		return &HostScan{
			HostID:        hostID,
			State:         StateTerminated,
			PeriodStart:   start,
			PeriodEnd:     end,
			ScanCompleted: true,
		}, nil
	}

	// Filter host usage to only include the current four-week period.
	usage := filterUsageByPeriod(host.RecentUsage, start, end)

	// If the current period usage is more than minimal usage,
	// pass until the next period begins. Don't need to check
	// how much time has passed since the beginning of period
	if len(usage) >= MinUsage {
		return &HostScan{
			HostID:      hostID,
			State:       StateActive,
			PeriodStart: start,
			PeriodEnd:   end,

			// Next scan should be at third week of the next period
			NextScan: pq.NullTime{end.Add(threeWeeks), true},
		}, nil
	}

	// Reaching here means user has not been active

	if timePassed < WarningPeriod {
		// We are still in the three weeks grace period.
		// This shouldn't happen often since next scan is
		// usually set to after the third week.
		// Would only happen for hosts that were somehow not
		// scanned previously
		return &HostScan{
			HostID:      hostID,
			State:       StateGracePeriod,
			PeriodStart: start,
			PeriodEnd:   end,

			// check again at the third week of current period
			NextScan: pq.NullTime{start.Add(threeWeeks), true},
		}, nil
	}

	if timePassed >= WarningPeriod && timePassed < TerminateNoticePeriod {
		// If we are within the third-fourth week period,
		// Send a one week notice before termination
		return &HostScan{
			HostID:      hostID,
			State:       StateOneWeekNotice,
			PeriodStart: start,
			PeriodEnd:   end,

			// check again at the last day of current period
			// to see whether to send last day notice
			NextScan: pq.NullTime{start.Add(TerminateNoticePeriod), true},
		}, nil
	}

	if timePassed >= TerminateNoticePeriod {
		// Otherwise we are in the terminate notice period
		// Notify the user that their host is being terminated
		return &HostScan{
			HostID:      hostID,
			State:       StateTerminating,
			PeriodStart: start,
			PeriodEnd:   end,

			// Terminate at the end of current period
			NextScan: pq.NullTime{end, true},
		}, nil
	}

	// Should never reach here.
	return nil, ErrUnknownScanState
}

func FillHostTransitions(d *db.Database, hosts []*ScannedHost, now time.Time) error {
	hostIDs := make([]int64, len(hosts))
	hostIndex := map[int64]*ScannedHost{}

	for i, host := range hosts {
		hostIDs[i] = host.ID
		hostIndex[host.ID] = host
	}

	usages, err := hosttransition.UsageListForHosts(d, hostIDs, now.Add(-HostScanPeriod), now)
	if err != nil {
		return err
	}

	for _, usage := range usages {
		host := hostIndex[usage.HostID]
		host.RecentUsage = append(host.RecentUsage, usage)
	}

	return nil
}

/*
 * Find list of free hosts that are ready for their next scan.
 * This is based on the latest next_scan field of a host, if
 * they have any entry in host_scans.
 *
 * The list excludes orphan and terminated hosts, as well as
 * hosts that have been upgraded to paid plan.
 *
 * Note: the list includes all hosts with hourly_price of 0.
 * to avoid terminating hosts with errorneous hourly_price set,
 * actual termination should make sure that the host owner's
 * billing profile is free before terminating.
 */
func FindScannableHosts(d *db.Database, now time.Time) ([]*ScannedHost, error) {
	hosts := []*ScannedHost{}
	err := d.Select(&hosts, `
		SELECT
			hosts.*, last_scan_state
		FROM hosts
		LEFT JOIN (
		  SELECT DISTINCT ON(host_id)
				host_id, next_scan, state AS last_scan_state, scan_completed
		  FROM host_scans
		  ORDER BY host_id, id DESC
		) AS last_scan
		  ON (last_scan.host_id = hosts.id)
		WHERE
		  hourly_price = 0
			AND terminated_at IS NULL
			AND preprovisioned = FALSE
			AND state != 'orphan'
			AND (scan_completed IS NULL OR scan_completed = FALSE)
			AND (next_scan IS NULL OR next_scan <= $1);
		`, now)

	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func FindScannableHostsWithUsage(d *db.Database, now time.Time) ([]*ScannedHost, error) {
	hosts, err := FindScannableHosts(d, now)
	if err != nil {
		return nil, err
	}

	err = FillHostTransitions(d, hosts, now)
	if err != nil {
		return nil, err
	}

	return hosts, nil
}

func ScanHosts(d *db.Database, now time.Time) ([]*HostScan, error) {
	scannedHosts, err := FindScannableHostsWithUsage(d, now)
	if err != nil {
		return nil, err
	}

	hostScans := make([]*HostScan, len(scannedHosts))
	for i, scannedHost := range scannedHosts {
		hostScans[i], err = ScanHost(scannedHost, now)
		if err != nil {
			return nil, err
		}
	}

	return hostScans, nil
}
