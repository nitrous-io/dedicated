package hostscan

import (
	"errors"
	"time"

	"github.com/lib/pq"
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
)

const (
	MinUsage = 4

	HostScanPeriod        = 4 * 7 * 24 * time.Hour
	WarningPeriod         = 3 * 7 * 24 * time.Hour
	TerminateNoticePeriod = (3*7 + 4) * 24 * time.Hour
)

var (
	ErrHostIsNotFree    = errors.New("Scanned host is not in free plan")
	ErrUnknownScanState = errors.New("Unable to determine scan state of host")
	ErrNotYetNextScan   = errors.New("Host is still not need to be scanned")
)

type ScanResult struct {
	Host     *host.Host
	State    string
	NextScan time.Time
}

func ScanHost(d *db.Database, host *host.Host, now time.Time) (*ScanResult, error) {
	if !host.Free {
		return nil, ErrHostIsNotFree
	}

	begin := *host.CreatedAt
	diff := now.Sub(begin)
	timePassed := diff % HostScanPeriod
	start := now.Add(-timePassed)
	end := start.Add(HostScanPeriod)

	usageCount, err := hosttransition.FindHostUsageCount(d, host.ID, start, end)
	if err != nil {
		return nil, err
	}

	if usageCount >= MinUsage {
		return &ScanResult{
			Host:     host,
			State:    StateActive,
			NextScan: end.Add(WarningPeriod),
		}, nil
	}

	if timePassed < WarningPeriod {
		return &ScanResult{
			Host:     host,
			State:    StateGracePeriod,
			NextScan: start.Add(WarningPeriod),
		}, nil
	}

	if timePassed >= WarningPeriod && timePassed < TerminateNoticePeriod {
		return &ScanResult{
			Host:     host,
			State:    StateOneWeekNotice,
			NextScan: start.Add(TerminateNoticePeriod),
		}, nil
	}

	if timePassed >= TerminateNoticePeriod {
		return &ScanResult{
			Host:     host,
			State:    StateTerminating,
			NextScan: end,
		}, nil
	}

	// Should never reach here.
	return nil, ErrUnknownScanState
}

func (scan *ScanResult) UpdateHost(d db.QueryReceiver) error {
	h := scan.Host

	if scan.State == StateTerminating {
		h.NextScan = pq.NullTime{}
		h.TerminatingAt = pq.NullTime{scan.NextScan, true}
	} else {
		h.NextScan = pq.NullTime{scan.NextScan, true}
	}

	return UpdateHostScanState(d, h)
}

func UpdateHostScanState(d db.QueryReceiver, h *host.Host) error {
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

func FindScannableHosts(d *db.Database, now time.Time) ([]*host.Host, error) {
	hosts := []*host.Host{}
	err := d.Select(&hosts, `
		SELECT * FROM hosts
		WHERE
			free IS TRUE
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
		SELECT hosts.*, pt.provider, pt.config FROM hosts
		INNER JOIN users ON (users.id = hosts.user_id)
		INNER JOIN billing_profiles ON (billing_profiles.user_id = users.id)
		INNER JOIN provider_templates AS pt ON hosts.provider_template_id = pt.id
		WHERE
			free IS TRUE
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
