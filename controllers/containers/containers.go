package containers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/lib/pq"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middlewares"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

// DefaultImage is the default Docker image used to create containers if an
// image isn't specified.
const DefaultImage = "nitrousio/ubuntu:trusty"

// FreighterClient is a shared freighterclient.Client for making requests to Freighter
// API hosts.
var FreighterClient freighterclient.Client

var (
	errSlugRequired      = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errHostIPUnknown     = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
	errHostNotFound      = httperror.New(http.StatusNotFound, "not_found", "host does not exist")
	errContainerNotFound = httperror.New(http.StatusNotFound, "not_found", "container does not exist")
	errInvalidPortspec   = httperror.New(http.StatusBadRequest, "invalid_request", "port spec invalid")
)

var (
	validations = map[string]error{
		"index_containers_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
)

var (
	Index   middlewares.Middleware = withHost(index)
	Create  middlewares.Middleware = withHost(create)
	Show    middlewares.Middleware = withContainer(show)
	Start   middlewares.Middleware = withContainer(start)
	Stop    middlewares.Middleware = withContainer(stop)
	Restart middlewares.Middleware = withContainer(restart)
	Destroy middlewares.Middleware = withContainer(destroy)
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
			if sc.CID == lc.ID {
				savedContainer = sc
				break
			}
		}
		if savedContainer != nil {
			hydrateContainer(savedContainer, lc, h)
			containers = append(containers, savedContainer)
		} else {
			containers = append(containers, mkContainer(lc, h))
		}
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

	slug := r.FormValue("slug")
	name := r.FormValue("name")
	image := r.FormValue("image")

	if image == "" {
		image = DefaultImage
	}

	if slug == "" {
		return errSlugRequired
	}

	if name == "" {
		name = slug
	}

	cntnr := &container.Container{
		UserID: env.User.ID,
		HostID: h.ID,
		Slug:   slug,
		Image:  image,
		Name:   sql.NullString{name, true},
	}

	// Save record to db first to reserve name (since it's globally unique).
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	err = cntnr.Insert(tx)
	if err != nil {
		tx.Rollback()
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" {
			if validationErr, ok := validations[e.Constraint]; ok {
				return validationErr
			}
		}
		return err
	}
	err = tx.Commit()
	if err != nil {
		return err
	}

	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}

	config, err := parseConfig(r)
	if err != nil {
		return err
	}

	created, err := FreighterClient.Create(fHost, slug, image, config)
	if err != nil {
		if e, ok := err.(*freighterclient.InvalidParamError); ok {
			return httperror.New(http.StatusBadRequest, "validation_failed", e.Msg)
		}
		return err
	}

	// Update existing record with new info from Freighter.
	cntnr.CID = created.ID
	cntnr.Image = created.Image
	cntnr.State = created.State
	cntnr.IPAddress = sql.NullString{created.Host, true}
	err = cntnr.Update(db)
	if err != nil {
		log.Errorf("containers.create: successfully created container '%+v' in Freighter but failed to update database with '%+v'", created, cntnr)
		// Continue responding as if nothing happened.
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status":    "created",
		"container": cntnr,
	}
	return nil
}

func show(cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	h, err := host.FindByID(db, cn.HostID)
	if err != nil {
		return err
	}
	if h == nil {
		return errHostNotFound
	}
	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}
	// TODO Run this in a goroutine so it runs concurrently with DB query.
	liveContainer, err := FreighterClient.Info(fHost, cn.CID)
	if err != nil {
		return err
	}
	hydrateContainer(cn, liveContainer, h)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": controllers.NewContainerAPI(cn, h),
	}

	return nil
}

func start(cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	return performControlAction(FreighterClient.Start, cn, env, c, w, r)
}

func stop(cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	return performControlAction(FreighterClient.Stop, cn, env, c, w, r)
}

func restart(cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	return performControlAction(FreighterClient.Restart, cn, env, c, w, r)
}

type actionFn func(h *freighterclient.Host, id string) (*freighterclient.Container, error)

func performControlAction(fn actionFn, cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	h, err := host.FindByID(db, cn.HostID)
	if err != nil {
		return err
	}
	if h == nil {
		return errHostNotFound
	}
	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}
	liveContainer, err := fn(fHost, cn.CID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": mkContainer(liveContainer, h),
	}

	return nil
}

func destroy(cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	h, err := host.FindByID(db, cn.HostID)
	if err != nil {
		return err
	}
	if h == nil {
		return errHostNotFound
	}
	fHost, err := mkFreighterHost(h)
	if err != nil {
		return err
	}
	liveContainer, err := FreighterClient.Terminate(fHost, cn.CID)
	if err != nil {
		return err
	}

	err = cn.Delete(db)
	if err != nil {
		return err
	}
	hydrateContainer(cn, liveContainer, h)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": cn,
	}
	return nil
}

func withHost(fn func(h *host.Host, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) middlewares.Middleware {
	return func(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		hSlug := r.FormValue("host")
		h, err := host.FindByUserAndSlug(db, env.User, hSlug)
		if err != nil {
			return err
		}
		if h == nil {
			return errHostNotFound
		}

		return fn(h, env, c, w, r)
	}
}

func withContainer(fn func(cn *container.Container, env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) middlewares.Middleware {
	return middlewares.Vars(func(env *middlewares.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		cSlug := env.Vars["slug"]
		cn, err := container.FindByUserAndSlug(db, env.User, cSlug)
		if err != nil {
			return err
		}
		if cn == nil {
			return errContainerNotFound
		}

		return fn(cn, env, c, w, r)
	})
}

func mkFreighterHost(h *host.Host) (*freighterclient.Host, error) {
	if h.IPAddress.String == "" {
		return nil, errHostIPUnknown
	}
	return freighterclient.NewHost(h.IPAddress.String, int(h.FreighterdPort.Int64), false), nil
}

func hydrateContainer(c *container.Container, fc *freighterclient.Container, h *host.Host) {
	c.CID = fc.ID
	c.Image = fc.Image
	c.Slug = dropNamespace(fc.Name)
	c.State = fc.State
	c.IPAddress = sql.NullString{fc.Host, true}
}

func mkContainer(fc *freighterclient.Container, h *host.Host) *container.Container {
	return &container.Container{
		CID:       fc.ID,
		Image:     fc.Image,
		Slug:      dropNamespace(fc.Name),
		State:     fc.State,
		IPAddress: sql.NullString{fc.Host, true},
		UserID:    h.UserID,
		HostID:    h.ID,
	}
}

func dropNamespace(freighterName string) string {
	parts := strings.SplitN(freighterName, ".", 2)
	switch len(parts) {
	case 1:
		return freighterName
	case 2:
		// more than one period is unexpected (disallowed in container slug)
		// not this func's concern, so pass-through after stripping portion
		// before first period
		return parts[1]
	default:
		return freighterName // never reached, satisfy compiler
	}
}

func parseConfig(r *http.Request) (freighterclient.ContainerConfig, error) {
	c := freighterclient.NewDockerContainerConfig()
	err := r.ParseForm()
	if err != nil {
		return nil, err
	}
	if envs, ok := r.PostForm["env"]; ok {
		c.Env = envs
	}
	if vols, ok := r.PostForm["volume"]; ok {
		c.Volumes = vols
	}
	if volsFrom, ok := r.PostForm["volumes_from"]; ok {
		c.VolumesFrom = volsFrom
	}
	if ports, ok := r.PostForm["port"]; ok {
		c.ExposedPorts = make([]string, 0, len(ports))
		c.PortBindings = make(map[string]string, len(ports))
		for _, spec := range ports {
			cport, binding, err := parsePortspec(spec)
			if err != nil {
				return nil, err
			}
			c.ExposedPorts = append(c.ExposedPorts, cport)
			c.PortBindings[cport] = binding
		}
	}
	return c, nil
}

// parsePortspec supports any of:
//   ip:hostPort:containerPort
//   ip::containerPort
//   hostPort:containerPort ... :
// returns (container port, ip:hostport, err)
func parsePortspec(portspec string) (string, string, error) {
	parts := strings.Split(portspec, ":")
	switch { // TODO more stringent checking
	case len(parts) == 3 && parts[1] == "": // ip::containerPort
		return parts[2], fmt.Sprintf("%s:%s", parts[0], parts[2]), nil
	case len(parts) == 3: // ip:hostPort:containerPort
		return parts[2], fmt.Sprintf("%s:%s", parts[0], parts[1]), nil
	case len(parts) == 2: // hostPort:containerPort
		return parts[1], fmt.Sprintf(":%s", parts[0]), nil
	default:
		return "", "", errInvalidPortspec
	}
}
