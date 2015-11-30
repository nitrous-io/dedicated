package snapshot

import (
	"database/sql"
	"regexp"
	"strings"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Snapshot struct {
	ID     int64         `db:"id"`
	UserID int64         `db:"user_id"`
	OrgID  sql.NullInt64 `db:"org_id"`

	Namespace     string `db:"namespace"`
	Name          string `db:"name"`
	Tag           string `db:"tag"`
	Repository    string `db:"repository"`
	DisplayName   string `db:"display_name"`
	OriginalImage string `db:"original_image"`

	CreatedAt pq.NullTime `db:"created_at"`
	UpdatedAt pq.NullTime `db:"updated_at"`
}

func FindAllByOrgID(d db.QueryReceiver, userID, orgID int64) ([]*Snapshot, error) {
	snapshots := []*Snapshot{}
	err := d.Select(&snapshots, `
		SELECT snapshots.*
		FROM snapshots
			JOIN orgs ON snapshots.org_id = orgs.id
			JOIN org_memberships ON orgs.id = org_memberships.org_id
		WHERE org_memberships.user_id = $1 AND snapshots.org_id = $2;`,
		userID, orgID)
	if err != nil {
		return nil, err
	}
	return snapshots, nil
}

var nitrousStackPattern = regexp.MustCompile(`nitrous\/([^:]+):`)
var stackPatterns = []struct {
	needle string
	stack  string
}{
	{"rails", "rails"},
	{"ruby", "ruby"},
	{"django", "django"},
	{"python", "python"},
	{"wordpress", "wordpress"},
	{"php", "php"},
	{"meteor", "meteor"},
	{"nodejs", "nodejs"},
	{"golang", "golang"},
	{"rust", "rust"},
	{"docker", "docker"},
	{"discourse", "discourse"},
	{"laravel", "laravel"},
	{"html5", "html5"},
}

func (s *Snapshot) Insert(d db.QueryReceiver) error {
	nstmt, err := d.PrepareNamed(`INSERT INTO snapshots
		(user_id, org_id, namespace, name, tag, repository, display_name, original_image)
		VALUES (:user_id, :org_id, :namespace, :name, :tag, :repository, :display_name, :original_image)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return nstmt.QueryRowx(s).StructScan(s)
}

func (s *Snapshot) Stack() string {
	if submatches := nitrousStackPattern.FindStringSubmatch(s.OriginalImage); submatches != nil {
		return submatches[1]
	}

	for _, sp := range stackPatterns {
		if strings.Contains(s.OriginalImage, sp.needle) {
			return sp.stack
		}
	}
	return ""
}
