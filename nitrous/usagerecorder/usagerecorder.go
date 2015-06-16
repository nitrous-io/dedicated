package usagerecorder

import (
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/hostusage"
)

type UsageRecorder struct {
	db        *db.Database
	startTime time.Time
	endTime   time.Time
}

type HostHourUsage struct {
	HostID     int64 `db:"host_id"`
	UserID     int64 `db:"user_id"`
	Terminated bool  `db:"terminated"`
	Seconds    int64 `db:"seconds"`
	// HourlyPrice unit is milli-cents (x1000)
	HourlyPrice int64 `db:"hourly_price"`
}

func (h *HostHourUsage) Hours() int {
	totalHours := (time.Duration(h.Seconds) * time.Second).Hours()
	return int(math.Ceil(totalHours))
}

func New(d *db.Database, startTime, endTime *time.Time) *UsageRecorder {
	base := &UsageRecorder{
		db: d,
	}
	if endTime == nil {
		base.endTime = time.Now()
	} else {
		base.endTime = *endTime
	}
	if startTime == nil {
		base.startTime = base.endTime.Add(time.Duration(-base.endTime.Hour()) * time.Hour).Truncate(time.Hour)
	} else {
		base.startTime = *startTime
	}
	return base
}

func (u *UsageRecorder) ProcessUsages() {
	logrus.Debugf("usage recorder: processing from %v to %v", u.startTime, u.endTime)
	lastId := int64(-1)
	usersExhausted := false

	// page through the hosts in the DB, 100 at a time
	for !usersExhausted {
		type con struct {
			Id int64 `db:"id"`
		}
		userIds := []con{}
		err := u.db.Select(&userIds, `SELECT id FROM users WHERE id > $1 ORDER BY id ASC LIMIT 100;`, lastId)
		if err != nil && err != sql.ErrNoRows {
			logrus.Errorf("usage recorder: user pager failed, err: %v", err)
		}

		if err == sql.ErrNoRows || len(userIds) == 0 {
			usersExhausted = true
			break
		}

		for _, userId := range userIds {
			if userId.Id > lastId {
				lastId = userId.Id
			}

			logrus.Debugf("usage recorder: calculating usage for %d", userId.Id)
			if err := u.RecordForUser(userId.Id); err != nil {
				logrus.Error(err)
			}
		}
	}
}

func (u *UsageRecorder) RecordForUser(uId int64) error {
	hostUsages, err := u.BillableHostHours(uId)
	if err != nil {
		return fmt.Errorf("usage recorder: failure fetching billable host hours for %d, err: %v", uId, err)
	}

	type exs struct {
		HostID int64 `db:"host_id"`
	}
	existing := []exs{}
	err = u.db.Select(&existing, "SELECT host_id FROM host_usages WHERE user_id = $1 AND day = $2",
		uId, u.startTime)
	if err != nil {
		return fmt.Errorf("usage recorder: failure establishing existing usages for %d, err: %v", uId, err)
	}

	tx, err := u.db.Beginx()
	if err != nil {
		return fmt.Errorf("usage recorder: failed to begin db transaction for %d, err: %v", uId, err)
	}

	for _, usage := range hostUsages {
		hU := &hostusage.HostUsage{
			HostID: usage.HostID,
			UserID: uId,
			Hours:  usage.Hours(),
			Day:    &u.startTime,
		}

		// decide based on existing if we need to perform an update OR insert
		op := hU.Insert
		for _, cH := range existing {
			if cH.HostID == usage.HostID {
				op = hU.Update
			}
		}

		if err := op(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("usage recorder: failed to persist HostUsage for %d, err: %v", usage.HostID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("usage recorder: failed to complete transaction for %d, err: %v", uId, err)
	}

	return nil
}

func (u *UsageRecorder) BillableHostHours(uId int64) ([]HostHourUsage, error) {
	hostHours := []HostHourUsage{}
	err := u.db.Select(&hostHours,
		`SELECT host_id,
			u.id user_id,
			COALESCE(MAX(h.terminated_at) <= $3, FALSE) terminated,
			MAX(h.hourly_price) hourly_price,
				cast((EXTRACT(EPOCH FROM
					SUM(COALESCE(ht.ended_at, $3) - GREATEST(ht.created_at, $2)))) as integer) seconds
			FROM host_transitions ht
			JOIN hosts h ON h.id = ht.host_id
			JOIN users u ON u.id = h.user_id

			WHERE u.id = $1
				AND ht.state = 'running'
				AND (ht.created_at, COALESCE(ht.ended_at, $3)) OVERLAPS ($2, $3)
			GROUP BY host_id, u.id;`,
		uId, u.startTime, u.endTime)
	return hostHours, err
}
