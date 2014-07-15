package containers

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

// FreighterClient is a shared freighterclient.Client for making requests to Freighter
// API hosts.
var FreighterClient freighterclient.Client

var (
	errImageRequired     = httperror.New(http.StatusBadRequest, "invalid_request", "image is required")
	errHostIPUnknown     = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
	errHostNotFound      = httperror.New(http.StatusNotFound, "not_found", "host does not exist")
	errContainerNotFound = httperror.New(http.StatusNotFound, "not_found", "container does not exist")
)

var (
	Index   middlewares.Middleware = withHost(index)
	Create  middlewares.Middleware = withHost(create)
	Show    middlewares.Middleware = withHost(withContainer(show))
	Start   middlewares.Middleware = withHost(withContainer(start))
	Stop    middlewares.Middleware = withHost(withContainer(stop))
	Restart middlewares.Middleware = withHost(withContainer(restart))
	Destroy middlewares.Middleware = withHost(withContainer(destroy))
)

func init() {
	FreighterClient = freighterclient.NewClient(nil)
}

func index(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	log := c.Logger()

	// FIXME Make Freighter API request and DB query concurrent.
	// 1. send request to Freighter API in goroutine.
	// 2. query DB normally.
	// 3. If db query returns err, stop other Freighter API goroutine (to avoid leaks) and return err.
	// 4. Wait for both db query and Freighter API goroutine to complete.
	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}
	liveContainers, err := FreighterClient.List(fHost)
	if err != nil {
		return err
	}

	db := c.DB()
	savedContainers, err := container.FindAllByHostID(db, h.ID)
	if err != nil {
		return err
	}

	log.Debug("Live containers:")
	for _, c := range liveContainers {
		log.Debugf("  %+v", c)
	}
	log.Debug("Saved containers:")
	for _, c := range savedContainers {
		log.Debugf("  %+v", c)
	}

	containers := []*container.Container{}
	for _, lc := range liveContainers {
		var savedContainer *container.Container
		for _, sc := range savedContainers {
			if sc.ID == lc.ID {
				savedContainer = sc
				break
			}
		}

		cntnr := hydrateContainer(lc, savedContainer, h)
		containers = append(containers, cntnr)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"containers": containers,
	}
	return nil
}

func create(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()
	log := c.Logger()

	name := r.FormValue("name")
	longname := r.FormValue("longname")
	image := r.FormValue("image")

	if image == "" {
		return errImageRequired
	}

	if name == "" {
		// TODO We should probably retry if the generated name ends up being
		// already in use.
		name = container.GenerateRandomName()
	}

	if longname == "" {
		longname = name
	}

	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}
	created, err := FreighterClient.Create(fHost, name, image)
	if err != nil {
		if e, ok := err.(*freighterclient.InvalidParamError); ok {
			return httperror.New(http.StatusBadRequest, "validation_failed", e.Msg)
		}
		return err
	}

	cntnr := &container.Container{
		ID:        created.ID,
		Image:     created.Image,
		Name:      created.Name,
		State:     created.State,
		IPAddress: sql.NullString{created.Host, true},
		LongName:  sql.NullString{longname, true},
		UserID:    env.User.ID,
		HostID:    h.ID,
	}

	// Save container to database without validations - since Freighter
	// successfuly created the container, we just want to save it.
	err = cntnr.Insert(db)
	if err != nil {
		log.Errorf("containers.create: successfully created container '%+v' in Freighter but failed to save to database as '%+v'", created, cntnr)
		// Continue responding as if nothing happened.
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status":    "created",
		"container": cntnr,
	}
	return nil
}

func show(h *host.Host, cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}
	// TODO Run this in a goroutine so it runs concurrently with DB query.
	liveContainer, err := FreighterClient.Info(fHost, cn.ID)
	if err != nil {
		return err
	}

	db := c.DB()
	savedContainer, err := container.FindByID(db, cn.ID)
	if err != nil {
		return err
	}

	cntnr := hydrateContainer(liveContainer, savedContainer, h)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": cntnr,
	}

	return nil
}

func start(h *host.Host, cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	return performControlAction(FreighterClient.Start, h, cn, env, c, w, r)
}

func stop(h *host.Host, cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	return performControlAction(FreighterClient.Stop, h, cn, env, c, w, r)
}

func restart(h *host.Host, cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	return performControlAction(FreighterClient.Restart, h, cn, env, c, w, r)
}

type actionFn func(h *freighterclient.Host, id string) (*freighterclient.Container, error)

func performControlAction(fn actionFn, h *host.Host, cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}
	liveContainer, err := fn(fHost, cn.ID)
	if err != nil {
		return err
	}

	cntnr := hydrateContainer(liveContainer, nil, h)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": cntnr,
	}

	return nil
}

func destroy(h *host.Host, cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {

	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}
	liveContainer, err := FreighterClient.Terminate(fHost, cn.ID)
	if err != nil {
		return err
	}

	db := c.DB()
	savedContainer, err := container.FindByID(db, cn.ID)
	if err != nil {
		return err
	}

	cntnr := hydrateContainer(liveContainer, savedContainer, h)
	now := time.Now()
	cntnr.TerminatedAt = &now
	err = cntnr.Update(db)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": cntnr,
	}
	return nil
}

func withHost(fn func(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) middlewares.Middleware {
	return middlewares.Vars(func(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		u := env.User
		if u == nil {
			return errors.New("user is nil")
		}

		hName := env.Vars["host"]
		h, err := host.FindByUserAndName(db, u, hName)
		if err != nil {
			return err
		}
		if h == nil {
			return errHostNotFound
		}

		return fn(h, env, c, w, r)
	})
}

type withHostMiddlewareFn func(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error

func withContainer(fn func(h *host.Host, cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) withHostMiddlewareFn {
	return func(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		cName := env.Vars["name"]
		cntnr, err := container.FindByHostIDAndName(db, h.ID, cName)
		if err != nil {
			return err
		}
		if cntnr == nil {
			return errContainerNotFound
		}

		return fn(h, cntnr, env, c, w, r)
	}
}

func mkFreighterHost(h *host.Host) (*freighterclient.Host, error) {
	if h.IPAddress.String == "" {
		return nil, errHostIPUnknown
	}
	return freighterclient.NewHost(h.IPAddress.String, int(h.FreighterdPort.Int64), false), nil
}

func hydrateContainer(fc *freighterclient.Container, cc *container.Container, h *host.Host) *container.Container {
	c := &container.Container{
		ID:        fc.ID,
		Image:     fc.Image,
		Name:      fc.Name,
		State:     fc.State,
		IPAddress: sql.NullString{fc.Host, true},
		UserID:    h.UserID,
		HostID:    h.ID,
	}
	if cc != nil {
		c.LongName = cc.LongName
		c.TerminatedAt = cc.TerminatedAt
		c.TouchedAt = cc.TouchedAt
		c.CreatedAt = cc.CreatedAt
		c.UpdatedAt = cc.UpdatedAt
	}
	return c
}
