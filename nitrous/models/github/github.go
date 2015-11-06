package github

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type GitHub struct {
	ID     int64 `db:"id"`
	UserID int64 `db:"user_id"`
	// AccessesAt stores a comma separated string containing unix timestamps
	// which record the time of access to the github endpoint
	// Eg. "1446606278,1446606279,1446606270"
	AccessesAt string `db:"accesses_at"`
	Threshold  int    `db:"threshold"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

func Exceeded(d db.QueryReceiver, uID int64) (bool, error) {
	g, err := FindByUserID(d, uID)
	g.UserID = uID
	if err != nil {
		if err != sql.ErrNoRows {
			return false, err
		}
		err = g.Insert(d, strconv.Itoa(int(time.Now().Unix())))
		return false, err
	}

	exceeded, err := g.track(d)
	if err != nil {
		return false, err
	}
	if exceeded {
		return true, nil
	}

	err = g.Update(d, strconv.Itoa(int(time.Now().Unix())))
	return false, err
}

func (g *GitHub) Insert(d db.QueryReceiver, el string) error {
	return d.QueryRowx(`INSERT INTO github
		(user_id, accesses_at)
		VALUES ($1, $2) 
		RETURNING *;`, g.UserID, el).StructScan(g)
}

func (g *GitHub) Update(d db.QueryReceiver, el string) error {
	nstmt, err := d.PrepareNamed(fmt.Sprintf(`UPDATE github 
		SET accesses_at
		= CONCAT_WS(',', accesses_at, %s)
		WHERE user_id = %d 
		RETURNING *;`, el, g.UserID))
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(g).StructScan(g)
}

func FindByUserID(d db.QueryReceiver, uID int64) (*GitHub, error) {
	g := &GitHub{}
	err := d.Get(g, `SELECT * FROM github WHERE user_id = $1;`, uID)
	return g, err
}

func (g *GitHub) track(d db.QueryReceiver) (bool, error) {
	var unexpiredTs []string
	timestamps, err := g.Accesses()
	if err != nil {
		return false, err
	}
	for _, ts := range timestamps {
		if ts > time.Now().Add(-time.Hour).Unix() {
			unexpiredTs = append(unexpiredTs, strconv.Itoa(int(ts)))
		}
	}
	accessesAt := strings.Join(unexpiredTs, ",")

	err = d.QueryRowx(`UPDATE github
		SET accesses_at = $1
		WHERE user_id = $2 
		RETURNING *;`, accessesAt, g.UserID).StructScan(g)
	if err != nil {
		return false, err
	}

	if len(unexpiredTs) > g.Threshold {
		return true, nil
	}

	return false, nil
}

func (g *GitHub) Accesses() ([]int64, error) {
	var t []int64
	timestamps := strings.Split(g.AccessesAt, ",")
	for _, ts := range timestamps {
		ps, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			return nil, err
		}
		t = append(t, ps)
	}
	return t, nil
}
