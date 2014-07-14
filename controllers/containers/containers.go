package containers

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/nitrous-io/nitrous-common/logger"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/freighter"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

// FreighterClient is a shared freighter.Client for making requests to Freighter
// API hosts.
var FreighterClient freighter.Client

var (
	errImageRequired = httperror.New(http.StatusBadRequest, "invalid_request", "image is required")
	errHostNotFound  = httperror.New(http.StatusNotFound, "not_found", "host does not exist")
	errHostIPUnknown = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
)

var (
	Index  middlewares.Middleware = withHost(index)
	Create middlewares.Middleware = withHost(create)
)

func init() {
	FreighterClient = freighter.NewClient(nil)
}

func index(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	log := c.Logger()

	// FIXME Make Freighter API request and DB query concurrent.
	// 1. send request to Freighter API in goroutine.
	// 2. query DB normally.
	// 3. If db query, stop other Freighter API goroutine (to avoid leaks) and return err.
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

		cntnr := hydrateContainer(lc, savedContainer, h, log)
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

	if name == "" {
		// TODO We should probably retry if the generated name ends up being
		// already in use.
		name = container.GenerateRandomName()
	}

	if image == "" {
		return errImageRequired
	}

	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}
	created, err := FreighterClient.Create(fHost, name, image)
	if err != nil {
		switch err := err.(type) {
		case freighter.InvalidParamError:
			return httperror.New(http.StatusBadRequest, "validation_failed", err.Msg)
		default:
			return err
		}
	}

	cntnr := &container.Container{
		ID:        created.ID,
		Image:     created.Image,
		Name:      created.Name,
		State:     created.State,
		IPAddress: sql.NullString{created.Host, true},
		UserID:    env.User.ID,
		HostID:    h.ID,
	}
	if longname != "" {
		cntnr.LongName = sql.NullString{longname, true}
	}

	err = cntnr.Validate()
	if err != nil {
		// TODO What should we do? Freighter successfully created the container,
		// but we can't save it to DB.
		return httperror.New(http.StatusBadRequest, "validation_failed", err.Error())
	}

	err = cntnr.Insert(db)
	if err != nil {
		// TODO What should we do? Freighter successfully created the container,
		// but we can't save it to DB.
		log.Errorf("containers.create: successfully created container '%+v' in Freighter but failed to save to database as '%+v'", created, cntnr)
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status":    "created",
		"container": cntnr,
	}
	return nil
}

func withHost(fn func(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) middlewares.Middleware {
	return func(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
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
	}
}

func mkFreighterHost(h *host.Host) (*freighter.Host, error) {
	if h.IPAddress.String == "" {
		return nil, errHostIPUnknown
	}
	return freighter.NewHost(h.IPAddress.String, int(h.FreighterdPort.Int64), false), nil
}

func hydrateContainer(fc *freighter.Container, cc *container.Container, h *host.Host, log *logger.Logger) *container.Container {
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
	} else {
		// It's possible that a container exists in Freighter but isn't
		// present in the database, e.g. container has been created by
		// Freighter but we have not been notified of its creation yet.
		// We just send as much info as possible.
		log.Warnf("Found container from Freighter that isn't stored in database: %v", fc)
	}
	return c
}
