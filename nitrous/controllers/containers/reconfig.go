package containers

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

func reconfig(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	log := env.Logger

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	hv, err := hostVolumes(db, h)
	if err != nil {
		return err
	}

	r.ParseForm()
	config, err := prepareConfig(r.PostForm, hv)
	if err != nil {
		return err
	}

	liveContainer, err := cl.Reconfig(cn.CID, config)
	if err != nil {
		return err
	}

	cn = hydrateContainer(cn, liveContainer, h)
	if err := cn.Update(db); err != nil {
		log.Errorf("containers.reconfig: successfully reconfigured container '%+v' in Freighter but failed to update database with '%+v', err: %v", liveContainer, cn, err)
		return err
	}

	if err := updateVolumes(db, cn, hv, volumeSlugs(config.Mounts)); err != nil { // log and continue
		log.Errorf("containers.reconfig: failed to update volumes on container %s, err: %v", cn.CID, err)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status":    "ok",
		"container": serializers.SerializeContainer(cn),
	}
	return nil
}
