package segment

import (
	"time"
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
