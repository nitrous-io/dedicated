package hosttransition

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/now"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

type HostTransition struct {
	ID          int64         `db:"id"`
	HostID      int64         `db:"host_id"`
	InitiatorID sql.NullInt64 `db:"initiator_id"`

	PreviousState sql.NullString `db:"previous_state"`
	State         string         `db:"state"`

	CreatedAt *time.Time `db:"created_at"`
	EndedAt   *time.Time `db:"ended_at"`
}

type HostUsage struct {
	HostID  int64     `db:"host_id"`
	Minutes int64     `db:"minutes"`
	Start   time.Time `db:"start_at"`
	End     time.Time `db:"end_at"`
}

func FindMostRecentForHost(db *db.Database, id int64) (*HostTransition, error) {
	hT := &HostTransition{}
	err := db.Get(hT, `SELECT * FROM host_transitions WHERE host_transitions.host_id = $1 ORDER BY created_at DESC LIMIT 1`, id)
	if err != nil {
		return nil, err
	}
	return hT, err
}

func FindByID(db *db.Database, id int64) (*HostTransition, error) {
	hT := &HostTransition{}
	err := db.Get(hT, `SELECT * FROM host_transitions WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	return hT, err
}

func hostsToHostIDs(hs []*host.Host) []int64 {
	hostIDs := make([]int64, len(hs))
	for i, h := range hs {
		hostIDs[i] = h.ID
	}
	return hostIDs
}

func joinInts(ids []int64) string {
	sids := make([]string, len(ids))
	for i, id := range ids {
		sids[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(sids, ", ")
}

func FindHostUsageCount(d *db.Database, hostID int64, start, end time.Time) (int64, error) {
	var usageCount int64

	err := d.QueryRow(`SELECT COUNT(*)
		FROM host_transitions
		WHERE host_id=$1
		AND state = 'running'
		AND (created_at, COALESCE(ended_at, $2)) OVERLAPS ($2, $3);
	`, hostID, start, end).Scan(&usageCount)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		} else {
			return 0, err
		}
	}

	return usageCount, nil
}

func UsageListForHosts(d db.QueryReceiver, hostIDs []int64, start, end time.Time) ([]*HostUsage, error) {
	// Skip query and return empty list if there is no host IDs provided
	if len(hostIDs) == 0 {
		return []*HostUsage{}, nil
	}

	query := fmt.Sprintf(`SELECT host_id,
		ceiling(
			cast(
				EXTRACT(
					EPOCH FROM
						LEAST(COALESCE(ended_at, $2), $2)
						- GREATEST(created_at, $1)
				)
				as integer
			) / 60.0
		) minutes,
		GREATEST(created_at, $1) start_at,
		LEAST(COALESCE(ended_at, $2), $2) end_at
		FROM host_transitions
		WHERE host_id IN (%s)
		AND state = 'running'
		AND (created_at, COALESCE(ended_at, $2)) OVERLAPS ($1, $2)
		ORDER BY id ASC;
	`, joinInts(hostIDs))

	usageList := []*HostUsage{}
	if err := d.Select(&usageList, query, start, end); err != nil {
		return nil, err
	}

	return usageList, nil
}

func UsageForHosts(d db.QueryReceiver, hostIDs []int64, start, end time.Time) (map[int64][]*HostUsage, error) {
	usageList, err := UsageListForHosts(d, hostIDs, start, end)
	if err != nil {
		return nil, err
	}

	res := make(map[int64][]*HostUsage)
	for _, r := range usageList {
		res[r.HostID] = append(res[r.HostID], r)
	}
	return res, nil
}

// Find total usage time for list of hosts
func totalUsageForHosts(d db.QueryReceiver, hostIDs []int64, start, end time.Time) (int64, error) {
	usageList, err := UsageListForHosts(d, hostIDs, start, end)
	if err != nil {
		return 0, err
	}

	var totalTime int64 = 0
	for _, usage := range usageList {
		totalTime += usage.Minutes
	}

	return totalTime, nil
}

func CurrentMonthUsageForHosts(d db.QueryReceiver, hostIDs []int64, rightNow time.Time) (int64, error) {
	start := now.New(rightNow).BeginningOfMonth()
	end := rightNow

	return totalUsageForHosts(d, hostIDs, start, end)
}

func CurrentMonthUsageForUser(d *db.Database, userID int64, now time.Time) (int64, []*host.Host, error) {
	hosts, err := host.FindAllWithTerminated(d, userID)
	if err != nil {
		return 0, nil, err
	}

	usage, err := CurrentMonthUsageForHosts(d, hostsToHostIDs(hosts), now)
	if err != nil {
		return 0, nil, err
	}

	return usage, hosts, nil
}

// Optional single initiatorID argument to record if the
// user initiating the host transition is an admin user
// instead of the host owner.
func Record(tx *sqlx.Tx, h *host.Host, previousState string, enactedAt time.Time, initiatorID int64) error {
	hT := &HostTransition{
		HostID:      h.ID,
		State:       h.State,
		CreatedAt:   &enactedAt,
		InitiatorID: sql.NullInt64{h.UserID, true},
	}

	if initiatorID > 0 {
		hT.InitiatorID = sql.NullInt64{initiatorID, true}
	}

	if previousState != "" {
		hT.PreviousState = sql.NullString{previousState, true}
	}

	var prevId int64
	err := tx.QueryRowx(
		`SELECT id FROM host_transitions WHERE host_transitions.host_id = $1 ORDER BY created_at DESC LIMIT 1`,
		h.ID).Scan(&prevId)

	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
	} else {
		// set previous
		_, err := tx.Exec(`UPDATE host_transitions SET ended_at = $1 WHERE id = $2;`, hT.CreatedAt, prevId)
		if err != nil {
			return err
		}
	}

	_, err = tx.NamedExec(
		`INSERT INTO host_transitions (host_id, initiator_id, previous_state, state, created_at)
      VALUES (:host_id, :initiator_id, :previous_state, :state, :created_at)`,
		hT)

	if err != nil {
		return err
	}

	if h.State == host.StateRunning {
		h.LastRunningAt = pq.NullTime{time.Now(), true}
		h.IdleNoticesSent = host.IDLE_NOTICES_NO_WARNING
		h.IdleNextCheck = pq.NullTime{Valid: false}
		nstmt, err := tx.PrepareNamed(`UPDATE hosts
                        SET last_running_at = :last_running_at,
                        idle_notices_sent = :idle_notices_sent,
                        idle_next_check = :idle_next_check
			WHERE id = :id RETURNING last_running_at;`)
		if err != nil {
			return err
		}
		return nstmt.QueryRowx(h).Scan(&h.LastRunningAt)
	}

	return nil
}
