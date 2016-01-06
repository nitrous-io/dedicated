package pendingmembers

import (
	"database/sql"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/orgmembership"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var errNotFound = httperror.New(http.StatusNotFound, "not_found", "could not find valid invite code")

func Show(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	code := mux.Vars(r)["code"]

	if code == "" {
		return errNotFound
	}

	orgMem, err := orgmembership.FindByInviteCode(db, code)
	if err == nil {
		env.Status = http.StatusOK
		mem := &orgmembership.OrgMember{
			OrgMembership: *orgMem,
		}
		env.Data = map[string]interface{}{
			"type":   "org",
			"member": serializers.SerializeOrgMember(mem),
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}

	collabMem, err := containercollab.FindByInviteCode(db, code)
	if err == nil {
		env.Status = http.StatusOK
		mem := &containercollab.ContainerCollabUser{
			ContainerCollab: *collabMem,
		}
		env.Data = map[string]interface{}{
			"type":   "collab",
			"member": serializers.SerializeContainerCollabUser(mem),
		}
		return nil
	}
	if err == sql.ErrNoRows {
		return errNotFound
	}
	return err
}
