package linkedin

import (
	"database/sql"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type LinkedinProfile struct {
	ID     int64  `db:"id"`
	UserID int64  `db:"user_id"`
	Header string `db:"header"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
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
