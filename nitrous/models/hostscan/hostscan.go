package hostscan

import (
	"errors"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
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
	MinUsage = 4

	oneDay     time.Duration = 24 * time.Hour
	oneWeek    time.Duration = 7 * oneDay
	threeWeeks time.Duration = 3 * oneWeek
	fourWeeks  time.Duration = 4 * oneWeek

	HostScanPeriod        = fourWeeks
	WarningPeriod         = HostScanPeriod - oneWeek
	TerminateNoticePeriod = HostScanPeriod - 3*oneDay
)

var (
	ErrHostIsNotFree    = errors.New("Scanned host is not in free plan")
	ErrUnknownScanState = errors.New("Unable to determine scan state of host")
	ErrNotYetNextScan   = errors.New("Host is still not need to be scanned")
)

type ScanResult struct {
	Host        *host.Host
	State       string
	PeriodStart time.Time
	PeriodEnd   time.Time
	NextScan    time.Time
}

func currentPeriod(begin, now time.Time, period time.Duration) (time.Time, time.Time, time.Duration) {
	diff := now.Sub(begin)
	timeSincePeriod := diff % period
	start := now.Add(-timeSincePeriod)
	end := start.Add(period)

	return start, end, timeSincePeriod
}

func ScanHost(d *db.Database, host *host.Host, now time.Time) (*ScanResult, error) {
	if host.HourlyPrice > 0 {
		return nil, ErrHostIsNotFree
	}

	start, end, timePassed := currentPeriod(*host.CreatedAt, now, fourWeeks)

	usageCount, err := hosttransition.FindHostUsageCount(d, host.ID, start, end)
	if err != nil {
		return nil, err
	}

	if usageCount >= MinUsage {
		return &ScanResult{
			Host:        host,
			State:       StateActive,
			PeriodStart: start,
			PeriodEnd:   end,
			NextScan:    end.Add(threeWeeks),
		}, nil
	}

	if timePassed < WarningPeriod {
		return &ScanResult{
			Host:        host,
			State:       StateGracePeriod,
			PeriodStart: start,
			PeriodEnd:   end,
			NextScan:    start.Add(threeWeeks),
		}, nil
	}

	if timePassed >= WarningPeriod && timePassed < TerminateNoticePeriod {
		return &ScanResult{
			Host:        host,
			State:       StateOneWeekNotice,
			PeriodStart: start,
			PeriodEnd:   end,
			NextScan:    start.Add(TerminateNoticePeriod),
		}, nil
	}

	if timePassed >= TerminateNoticePeriod {
		return &ScanResult{
			Host:        host,
			State:       StateTerminating,
			PeriodStart: start,
			PeriodEnd:   end,
			NextScan:    end,
		}, nil
	}

	// Should never reach here.
	return nil, ErrUnknownScanState
}

func FindScannableHosts(d *db.Database, now time.Time) ([]*host.Host, error) {
	hosts := []*host.Host{}
	err := d.Select(&hosts, `
		SELECT * FROM hosts
		WHERE
		  hourly_price = 0
			AND terminated_at IS NULL
			AND preprovisioned = FALSE
			AND state != 'orphan'
			AND scan_exempted = FALSE
			AND terminating_at IS NULL
			AND (next_scan IS NULL OR next_scan <= $1);
		`, now)

	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func FindTerminatingHosts(d *db.Database, now time.Time) ([]*host.Host, error) {
	hosts := []*host.Host{}
	err := d.Select(&hosts, `
		SELECT hosts.* FROM hosts
		INNER JOIN users ON (users.id = hosts.user_id)
		INNER JOIN billing_profiles ON (billing_profiles.user_id = users.id)
		WHERE
		  hourly_price = 0
			AND terminated_at IS NULL
			AND preprovisioned = FALSE
			AND hosts.state != 'orphan'
			AND scan_exempted = FALSE
			AND billing_profiles.state = $1
			AND terminating_at <= $2;
		`, billingprofile.StateFree, now)

	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func UpdateHostScanState(d *db.Database, h *host.Host) error {
	nstmt, err := d.PrepareNamed(`UPDATE hosts
		SET (next_scan, terminating_at, updated_at)
		= (:next_scan, :terminating_at, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func UpdateHostScanExempt(d *db.Database, h *host.Host) error {
	nstmt, err := d.PrepareNamed(`UPDATE hosts
		SET (scan_exempted, updated_at)
		= (:scan_exempted, now())
		WHERE id = :id
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(h).StructScan(h)
}

func ScanHosts(d *db.Database, now time.Time) ([]*ScanResult, []error) {
	errors := []error{}
	hostScans := []*ScanResult{}

	scannedHosts, err := FindScannableHosts(d, now)
	if err != nil {
		errors = append(errors, err)
		return hostScans, errors
	}

	for _, scannedHost := range scannedHosts {
		result, err := ScanHost(d, scannedHost, now)
		if err != nil {
			errors = append(errors, err)
		} else {
			hostScans = append(hostScans, result)
		}
	}

	return hostScans, errors
}
