package snapshots

import (
	"database/sql"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/org"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/models/snapshot"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errOrgNotFound  = httperror.New(http.StatusNotFound, "not_found", "org does not exist")
	errNotOrgMember = httperror.New(http.StatusForbidden, "forbidden", "you are not a member of this org")
)

func ForOrg(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	orgSlug := mux.Vars(r)["org_slug"]
	o, err := org.FindBySlug(db, orgSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errOrgNotFound
		}
	}

	_, err = orgmembership.FindByUsernameAndOrgSlug(db, env.User.Username, orgSlug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errNotOrgMember
		}
		return err
	}

	snapshots, err := snapshot.FindAllByOrgID(db, env.User.ID, o.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"snapshots": serializers.SerializeSnapshots(snapshots),
	}

	return nil
}
