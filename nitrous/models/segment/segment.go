package segment

import (
	"time"

	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Segment struct {
	ID int64 `db:"id"`

	Slug string `db:"slug"`

	CreatedAt *time.Time `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`

	UserDefault     bool `db:"user_default"`
	OrgDefault      bool `db:"org_default"`
	FreeUserDefault bool `db:"free_user_default"`
}

func FreeUserDefaultID(d db.QueryReceiver) (int64, error) {
	var id int64
	if err := d.QueryRowx(`SELECT id from segments where free_user_default=true`).Scan(&id); err != nil {
		return 0, err
	}

	return id, nil
}

func UserDefaultID(d db.QueryReceiver) (int64, error) {
	var id int64
	if err := d.QueryRowx(`SELECT id from segments where user_default=true`).Scan(&id); err != nil {
		return 0, err
	}

	return id, nil
}
