package plan_segment

import (
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type PlanSegment struct {
	ID        int64 `db:"id"`
	SegmentID int64 `db:"segment_id"`
	PlanID    int64 `db:"plan_id"`
}

func (ps *PlanSegment) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO plan_segments (segment_id, plan_id) VALUES (:segment_id, :plan_id) RETURNING *;`)
	if err != nil {
		return err
	}

	return nstmt.QueryRowx(ps).StructScan(ps)
}
