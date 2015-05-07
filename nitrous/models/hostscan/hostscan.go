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

type HostScan struct {
	ID             int64         `db:"id"`
	HostID         int64         `db:"host_id"`
	State          string        `db:"state"`
	AnnouncementID sql.NullInt64 `db:"announcement_id"`
	PeriodStart    time.Time     `db:"period_start"`
	PeriodEnd      time.Time     `db:"period_end"`
	RecordedAt     time.Time     `db:"recorded_at"`
	NextScan       pq.NullTime   `db:"next_scan"`
	ScanCompleted  bool          `db:"scan_completed"`
}

type ScannedHost struct {
	*host.Host
	LastScanState sql.NullString `db:"last_scan_state"`
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

func currentPeriod(begin, now time.Time, period time.Duration) (time.Time, time.Time, time.Duration) {
	diff := now.Sub(begin)
	timeSincePeriod := diff % period
	start := now.Add(-timeSincePeriod)
	end := start.Add(period)

	return start, end, timeSincePeriod
}

func ScanHost(d *db.Database, host *ScannedHost, now time.Time) (*HostScan, error) {
	hostID := host.ID
	hostCreated := *host.CreatedAt

	if host.HourlyPrice > 0 {
		return nil, ErrHostIsNotFree
	}

	start, end, timePassed := currentPeriod(hostCreated, now, fourWeeks)

	if host.LastScanState.Valid && host.LastScanState.String == StateTerminating {
		return &HostScan{
			HostID:        hostID,
			State:         StateTerminated,
			PeriodStart:   start,
			PeriodEnd:     end,
			ScanCompleted: true,
		}, nil
	}

	usageCount, err := hosttransition.FindHostUsageCount(d, host.ID, start, end)
	if err != nil {
		return nil, err
	}

	if usageCount >= MinUsage {
		return &HostScan{
			HostID:      hostID,
			State:       StateActive,
			PeriodStart: start,
			PeriodEnd:   end,

			NextScan: pq.NullTime{end.Add(threeWeeks), true},
		}, nil
	}

	if timePassed < WarningPeriod {
		return &HostScan{
			HostID:      hostID,
			State:       StateGracePeriod,
			PeriodStart: start,
			PeriodEnd:   end,
			NextScan:    pq.NullTime{start.Add(threeWeeks), true},
		}, nil
	}

	if timePassed >= WarningPeriod && timePassed < TerminateNoticePeriod {
		return &HostScan{
			HostID:      hostID,
			State:       StateOneWeekNotice,
			PeriodStart: start,
			PeriodEnd:   end,
			NextScan:    pq.NullTime{start.Add(TerminateNoticePeriod), true},
		}, nil
	}

	if timePassed >= TerminateNoticePeriod {
		return &HostScan{
			HostID:      hostID,
			State:       StateTerminating,
			PeriodStart: start,
			PeriodEnd:   end,
			NextScan:    pq.NullTime{end, true},
		}, nil
	}

	// Should never reach here.
	return nil, ErrUnknownScanState
}

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

func ScanHosts(d *db.Database, now time.Time) ([]*HostScan, error) {
	scannedHosts, err := FindScannableHosts(d, now)
	if err != nil {
		return nil, err
	}

	hostScans := make([]*HostScan, len(scannedHosts))
	for i, scannedHost := range scannedHosts {
		hostScans[i], err = ScanHost(d, scannedHost, now)
		if err != nil {
			return nil, err
		}
	}

	return hostScans, nil
}
