package containers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/lib/pq"
	"github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/container"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

// DefaultImage is the default Docker image used to create containers if an
// image isn't specified.
const DefaultImage = "nitrousio/ubuntu:trusty"

var (
	errSlugRequired      = httperror.New(http.StatusBadRequest, "invalid_request", "slug is required")
	errHostNotRunning    = httperror.New(http.StatusBadRequest, "invalid_state", "host is not running") // Not 100% on the status code - how about 409 Conflict?
	errHostNotFound      = middleware.ErrHostNotFound
	errContainerNotFound = httperror.New(http.StatusNotFound, "not_found", "container does not exist")
	errInvalidPortspec   = httperror.New(http.StatusBadRequest, "invalid_request", "port spec invalid")
)

var (
	validations = map[string]error{
		"index_containers_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
)

var (
	Index   middleware.Middleware = middleware.HostViaForm("host", index)
	Create  middleware.Middleware = middleware.HostViaForm("host", create)
	Show    middleware.Middleware = WithContainerAndHost(show)
	Start   middleware.Middleware = WithContainerAndHost(start)
	Stop    middleware.Middleware = WithContainerAndHost(stop)
	Restart middleware.Middleware = WithContainerAndHost(restart)
	Destroy middleware.Middleware = WithContainerAndHost(destroy)
)

var mkFreighterClient = func(h *host.Host, c *cert.Cert) (freighterclient.Client, error) {
	if h.IPAddress.String == "" {
		return nil, errHostIPUnknown
	}
	hst := freighterclient.NewHost(
		h.IPAddress.String,
		int(h.FreighterdPort.Int64),
		fmt.Sprintf("%s.nitrouspro.com", h.Slug),
		[]byte(c.CA),
		[]byte(c.ClientCert),
		[]byte(c.ClientKey),
	)
	return freighterclient.NewClient(hst)
}

func index(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	if h.State != host.StateRunning {
		return errHostNotRunning
	}

	db := c.DB()
	crt, err := cert.Get(c.DB(), h.ID)
	if err != nil {
		return err
	}

	log := c.Logger()

	out := make(chan listresult, 1)
	quit := make(chan struct{})
	go freighterList(h, crt, out, quit)

	savedContainers, err := container.FindAllByHostID(db, h.ID)
	if err != nil {
		close(quit)
		return err
	}

	live := <-out
	if live.err != nil {
		return live.err
	}

	log.Debug("Live containers:")
	for _, c := range live.containers {
		log.Debugf("  %+v", c)
	}
	log.Debug("Saved containers:")
	for _, c := range savedContainers {
		log.Debugf("  %+v", c)
	}

	containers := []*container.Container{}
	for _, lc := range live.containers {
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

type listresult struct {
	containers []*freighterclient.Container
	err        error
}

func freighterList(h *host.Host, crt *cert.Cert, out chan<- listresult, quit <-chan struct{}) {
	select {
	case <-quit:
		return
	default:
		cl, err := mkFreighterClient(h, crt)
		if err != nil {
			out <- listresult{err: err}
			return
		}
		liveContainers, err := cl.List()
		if err != nil {
			out <- listresult{err: err}
			return
		}
		out <- listresult{containers: liveContainers, err: nil}
	}
}

func create(h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	if h.State != host.StateRunning {
		return errHostNotRunning
	}

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

	crt, err := cert.Get(db, h.ID)
	if err != nil {
		return err
	}

	cl, err := mkFreighterClient(h, crt)
	if err != nil {
		return err
	}

	config, err := parseConfig(r)
	if err != nil {
		return err
	}

	created, err := cl.Create(slug, image, config)
	if err != nil {
		delErr := cntnr.DeletePermanently(db)
		if delErr != nil {
			c.Logger().Errorf("Unable to delete container %d that failed on freighter: %v", cntnr.ID, delErr)
		}
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

func show(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	crt, err := cert.Get(c.DB(), h.ID)
	if err != nil {
		return err
	}

	cl, err := mkFreighterClient(h, crt)
	if err != nil {
		return err
	}

	// TODO Run this in a goroutine so it runs concurrently with DB query.
	liveContainer, err := cl.Info(cn.CID)
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

func start(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	crt, err := cert.Get(c.DB(), h.ID)
	if err != nil {
		return err
	}

	cl, err := mkFreighterClient(h, crt)
	if err != nil {
		return err
	}
	return performControlAction(cl.Start, cn, h, env, c, w, r)
}

func stop(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	crt, err := cert.Get(c.DB(), h.ID)
	if err != nil {
		return err
	}

	cl, err := mkFreighterClient(h, crt)
	if err != nil {
		return err
	}
	return performControlAction(cl.Stop, cn, h, env, c, w, r)
}

func restart(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	crt, err := cert.Get(c.DB(), h.ID)
	if err != nil {
		return err
	}
	cl, err := mkFreighterClient(h, crt)
	if err != nil {
		return err
	}
	return performControlAction(cl.Restart, cn, h, env, c, w, r)
}

type actionFn func(id string) (*freighterclient.Container, error)

func performControlAction(fn actionFn, cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	liveContainer, err := fn(cn.CID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": mkContainer(liveContainer, h),
	}

	return nil
}

func destroy(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	db := c.DB()

	crt, err := cert.Get(db, h.ID)
	if err != nil {
		return err
	}

	cl, err := mkFreighterClient(h, crt)
	if err != nil {
		return err
	}

	liveContainer, err := cl.Terminate(cn.CID)
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

func WithContainerAndHost(fn func(cn *container.Container, h *host.Host, env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error) middleware.Middleware {
	return middleware.Vars(func(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
		db := c.DB()

		cSlug := env.Vars["slug"]
		cn, err := container.FindByUserAndSlug(db, env.User, cSlug)
		if err != nil {
			return err
		}
		if cn == nil {
			return errContainerNotFound
		}

		h, err := host.FindByID(db, cn.HostID)
		if err != nil {
			return err
		}
		if h == nil {
			return errHostNotFound
		}
		if h.State != host.StateRunning {
			return errHostNotRunning
		}

		return fn(cn, h, env, c, w, r)
	})
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
