package containers

import (
	"database/sql"
	"net/http"

	"github.com/lib/pq/hstore"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

var (
	Metadata = WithContainerAndHost(metadata, true, false)
)

func metadata(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	if err := r.ParseForm(); err != nil {
		return err
	}

	curStore := cn.Metadata
	if curStore.Map == nil {
		cn.Metadata = hstore.Hstore{
			Map: map[string]sql.NullString{},
		}
		curStore = cn.Metadata
	}
	for key, value := range r.Form {
		if value[0] == "" {
			delete(curStore.Map, key)
		} else if len(key) <= 255 {
			curStore.Map[key] = sql.NullString{value[0], true}
		}
	}

	nstmt, err := db.PrepareNamed(`UPDATE containers
    SET (metadata, updated_at)
    = (:metadata, now())
    WHERE id = :id
    RETURNING *;`)
	if err != nil {
		return err
	}
	if err := nstmt.QueryRowx(cn).StructScan(cn); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}
