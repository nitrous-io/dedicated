package containers

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	ndb "github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/cvolume"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

const reconfigLockTime = 2 * time.Minute

var (
	Reconfig = WithContainerAndHost(reconfig, TouchHost, AllowCollab)
)

func reconfig(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	hv, err := hostVolumes(db, h)
	if err != nil {
		return err
	}

	r.ParseForm()
	config := &freighterclient.ContainerConfig{}
	if err := prepareConfig(config, r.PostForm, hv); err != nil {
		return err
	}

	ctx := &reconfigContext{
		db:     db,
		cn:     cn,
		h:      h,
		hv:     hv,
		config: config,
	}
	async, _ := strconv.ParseBool(r.FormValue("async"))

	err = doContainerSyncOrAsyncExclusive(
		async, cn, env,
		container.StateReconfiguring, reconfigLockTime,
		func() error {
			PublishContainerEvent(env, controllers.EventContainerReconfiguring, cn, nil)

			err := reconfigureContainer(ctx, env)
			if err != nil {
				_ = cn.Update(db) // Ignore error here.
				errMsg := err.Error()
				if e, ok := err.(*httperror.Error); ok {
					errMsg = e.Description
				}
				add := map[string]interface{}{
					"error": errMsg,
				}
				PublishContainerEvent(env, controllers.EventContainerReconfigureFailed, cn, add)
				return err
			}

			return nil
		})

	if err != nil {
		return err
	}

	if async {
		env.Status = http.StatusAccepted
	} else {
		env.Status = http.StatusOK
	}

	/*
	 * Note: the container cn here may or may not
	 * be hydrated depend on whether it is sync or async.
	 */
	env.Data = map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
	}
	return nil
}

type reconfigContext struct {
	config *freighterclient.ContainerConfig
	cn     *container.Container
	h      *host.Host
	hv     map[string]*volume.Volume

	db *ndb.Database
}

func reconfigureContainer(ctx *reconfigContext, env *middleware.Env) (err error) {
	cn, h, hv, db := ctx.cn, ctx.h, ctx.hv, ctx.db
	config := ctx.config

	var cl *controllers.FreighterClient
	cl, err = controllers.NewFreighterClient(db, h, env.UUID)
	if err != nil {
		return err
	}

	var liveContainer *freighterclient.Container
	liveContainer, err = cl.Reconfig(cn.CID, config)
	if err != nil {
		if liveContainer != nil {
			cn.TempState = sql.NullInt64{0, false}
			cn = hydrateContainer(cn, liveContainer, h)
		}
		err = expandFreighterClientError(db, cn, err)
		return err
	}

	// reset the temp state, so freigther state will be exposed
	cn.TempState = sql.NullInt64{0, false}

	cn = hydrateContainer(cn, liveContainer, h)

	var newConfig *freighterclient.ContainerConfig
	newConfig, err = cl.Config(cn.CID)
	if err != nil {
		return err
	}

	logger := logrus.WithFields(logrus.Fields{
		"uuid":           env.UUID,
		"container_slug": cn.Slug,
	})

	// Update port mappings
	portMappings, err := GetPortMappingsJsonFrom(liveContainer)
	if err == nil {
		cn.PortMappings = portMappings
	} else if err != nil {
		logger.Errorf("containers.reconfig: Failed to get preview port: %+v", err)
	}

	err = cn.Update(db)
	if err != nil {
		logger.Errorf("containers.reconfig: successfully reconfigured container '%+v' in Freighter but failed to update database with '%+v', err: %v", liveContainer, cn, err)
		return err
	}

	if err = updateVolumes(db, cn, hv, volumeSlugs(config.Mounts)); err != nil { // log and continue
		logger.Errorf("containers.reconfig: failed to update volumes on container '%+v', err: %v", cn, err)
	}

	add := map[string]interface{}{
		"config": serializers.SerializeContainerConfig(newConfig),
	}

	PublishContainerEvent(env, controllers.EventContainerReconfigured, cn, add)

	return nil
}

func updateVolumes(d *ndb.Database, cn *container.Container, hv map[string]*volume.Volume, slugs []string) error {
	tx, err := d.Beginx()
	if err != nil {
		return err
	}
	if err := cvolume.DeleteContainer(tx, cn); err != nil {
		tx.Rollback()
		return err
	}
	for _, v := range slugs {
		cv := cvolume.ContainerVolume{ContainerID: cn.ID, VolumeID: hv[v].ID}
		if err := cv.Insert(tx); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
