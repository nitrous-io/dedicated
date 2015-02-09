package containers

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/shellwords"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/nitrous-common/hmactoken"
)

const defaultLockTime = 1 * time.Minute

var (
	ignoredImageNames = regexp.MustCompile(`-reconfig-\d{19}\z`) // reconfig names include Unix epoch (nano) stamp
	bindForMessage    = regexp.MustCompile(`Bind\s+for\s+0.0.0.0:(\d+)`)
)

var (
	errImageNameRequired = httperror.New(http.StatusBadRequest, "invalid_request", "image name is required")
	errHostNotFound      = middleware.ErrHostNotFound
	errVolumeNotFound    = httperror.New(http.StatusBadRequest, "invalid_state", "volume not found")
	errContainerNotFound = httperror.New(http.StatusNotFound, "container_not_found", "container does not exist")
	errInvalidPortspec   = httperror.New(http.StatusBadRequest, "invalid_request", "port spec invalid")
	errInvalidCommand    = httperror.New(http.StatusBadRequest, "invalid_request", "container cmd invalid")
	errContainerLocked   = httperror.New(http.StatusConflict, "container_locked", "container is locked")

	makeErrHostPortConflict = func(port string) error {
		return httperror.New(http.StatusConflict, "host_port_in_use", fmt.Sprintf("Container cannot be started, Host port %s is already in use by another container.", port))
	}
)

var (
	validations = map[string]error{
		"index_containers_on_slug": httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken"),
	}
)

var (
	Create  = middleware.HostViaForm("host", requiresRunningHost(create))
	Commit  = WithContainerAndHost(commit)
	Config  = WithContainerAndHost(config)
	Show    = WithContainerAndHost(show)
	Start   = WithContainerAndHost(start)
	Stop    = WithContainerAndHost(stop)
	Restart = WithContainerAndHost(restart)
	Destroy = WithContainerAndHost(destroy)
	Token   = WithContainerAndHost(token)
)

var (
	PerformCreateFn = CreateContainer
)

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	var err error

	hSlug := r.FormValue("host")
	hosts := []host.Host{}
	containersResp := []*serializers.Container{}

	// If no host is provided, then get all hosts belonging to the user
	// and get all containers from all hosts
	if hSlug != "" {
		h, err := host.Find(db, env.User, hSlug)
		if err != nil {
			return err
		}

		if h != nil {
			hosts = []host.Host{*h}
		} else {
			return errHostNotFound
		}
	} else {
		hosts, err = host.FindAllByUser(db, env.User)
		if err != nil {
			return err
		}
	}

	errorMessages := []string{}

	for _, h := range hosts {
		liveContainers, freighterErr := freighterList(db, &h)

		savedContainers, err := container.FindAllByHostID(db, h.ID)
		if err != nil {
			env.Logger.Errorf("unable to find all containers: %v, for host %s", err, h.Slug)
			errorMessages = append(errorMessages, fmt.Sprintf("Unable to locate containers for %s", h.Slug))
		}

		if freighterErr != nil {
			// If we can't get through to the host, then pass them along without the Freighter specific
			// info to the frontend
			if freighterErr == controllers.ErrHostNotRunning || freighterErr == controllers.ErrHostIPUnknown {
				for _, sc := range savedContainers {
					sc.State = container.StateHostStopped
					containersResp = append(containersResp, serializers.SerializeContainer(sc))
				}
			} else {
				env.Logger.Errorf("freighter err is not nil: %v, for host %s", freighterErr, h.Slug)
				if strings.HasPrefix(freighterErr.Error(), "freighter:") {
					errorMessages = append(errorMessages, fmt.Sprintf("Unable to communicate with freighter for host %s", h.Slug))
				} else {
					errorMessages = append(errorMessages, fmt.Sprintf("Unable to locate containers for host %s", h.Slug))
				}
				continue
			}
		}

		for _, lc := range liveContainers {
			var savedContainer *container.Container
			for _, sc := range savedContainers {
				if sc.CID == lc.ID {
					savedContainer = sc
					break
				}
			}
			var cn *container.Container
			if savedContainer != nil {
				cn = hydrateContainer(savedContainer, lc, &h)
			} else {
				cn = mkContainer(lc, &h)
			}

			containersResp = append(containersResp, serializers.SerializeContainer(cn))
		}
	}

	sort.Sort(serializers.BySlug(containersResp))
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"containers": containersResp,
	}
	if len(errorMessages) != 0 {
		env.Data["errors"] = errorMessages
	}
	return nil
}

type listresult struct {
	containers []*freighterclient.Container
	err        error
}

func freighterList(db db.QueryReceiver, h *host.Host) ([]*freighterclient.Container, error) {
	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return nil, err
	}
	liveContainers, err := cl.List()
	if err != nil {
		return nil, err
	}
	return liveContainers, nil
}

func create(h runningHost, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}
	res, err := PerformCreateFn(h, r.Form, env)
	if err != nil {
		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"container": res.Container,
	}
	if res.ErrorMessage != "" {
		env.Data["error"] = res.ErrorMessage
		env.Data["error_description"] = res.ErrorDescription
	}
	return nil
}

func commit(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	name := r.FormValue("name")
	if name == "" {
		return errImageNameRequired
	}

	created, err := cl.Commit(cn.CID, name)
	if err == freighterclient.ErrImageExists {
		env.Status = http.StatusBadRequest
		env.Data = map[string]interface{}{
			"error":             "invalid_request",
			"error_description": "image already exists",
		}
		return nil
	}
	if err == freighterclient.ErrIllegalTagName {
		env.Status = http.StatusBadRequest
		env.Data = map[string]interface{}{
			"error":             "invalid_request",
			"error_description": "Illegal tag name: only [A-Za-z0-9_.-] are allowed, minimum 1, maximum 128 in length",
		}
		return nil
	}
	if err != nil {
		return err
	}

	var builtAt *time.Time
	if created.CreatedAt != 0 {
		t := time.Unix(created.CreatedAt, 0).UTC()
		builtAt = &t
	}

	img := &image.Image{
		UserID:      env.User.ID,
		HostID:      h.ID,
		FreighterID: created.ID,
		Slug:        created.Name,
		Tag:         created.Tag,
		ActualSize:  created.ActualSize,
		VirtualSize: created.VirtualSize,
		BuiltAt:     builtAt,
	}
	if err := img.InsertOrUpdate(env.DB); err != nil { // log and continue
		env.Logger.Errorf("containers.create: failed to insert or update image '%+v'", img)
	}

	sImg := serializers.SerializeImage(img)
	liveContainer, err := cl.Info(cn.CID)
	if err != nil {
		return err
	}
	cn = hydrateContainer(cn, liveContainer, h)

	env.Events.Publish(controllers.NewEvent(controllers.EventImageCreated, map[string]*serializers.Image{
		"image": sImg,
	}), env.User.Topic())
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"image":     sImg,
		"container": serializers.SerializeContainer(cn),
	}
	return nil
}

// TODO: deprecate eventually once webapp is updated, as `show` includes config now
func config(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	config, err := cl.Config(cn.CID)
	if err != nil {
		return err
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"config": serializers.SerializeContainerConfig(config),
	}

	return nil
}

func show(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		if err == controllers.ErrHostNotRunning || err == controllers.ErrHostIPUnknown {
			cn.State = container.StateHostStopped

			env.Status = http.StatusOK
			env.Data = map[string]interface{}{
				"container": serializers.SerializeContainer(cn),
			}
			return nil
		} else {
			return err
		}
	}

	// TODO Run this in a goroutine so it runs concurrently with DB query.
	liveContainer, err := cl.Info(cn.CID)
	if err != nil {
		return err
	}
	cn = hydrateContainer(cn, liveContainer, h)

	config, err := cl.Config(cn.CID)
	if err != nil {
		return err
	}

	serialized := serializers.SerializeContainer(cn)
	serialized.Config = serializers.SerializeContainerConfig(config)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": serialized,
	}

	return nil
}

func start(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hv, err := hostVolumes(env.DB, h)
	if err != nil {
		return err
	}
	r.ParseForm()
	config, err := prepareConfig(r.PostForm, hv)
	if err != nil {
		return err
	}

	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}

	if err := cn.Lock(container.StateStarting, defaultLockTime, env.DB); err != nil {
		if err == container.ErrLocked {
			return errContainerLocked
		}
		return err
	}
	defer cn.Unlock(env.DB)

	// broadcast transitioning state
	publishContainerEvent(env, controllers.EventContainerStarting, cn)

	liveContainer, err := cl.Start(cn.CID, config)
	if err != nil {
		return expandFreighterClientError(err)
	}
	// reset the temp state, so freigther state will be exposed
	cn.TempState = sql.NullInt64{0, false}
	cn = hydrateContainer(cn, liveContainer, h)

	publishContainerEvent(env, controllers.EventContainerStarted, cn)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
	}
	return nil
}

func stop(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}
	return performControlAction(cl.Stop, container.StateStopping, controllers.EventContainerStopped, cn, h, env, w, r)
}

func restart(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h)
	if err != nil {
		return err
	}
	return performControlAction(cl.Restart, container.StateRestarting, controllers.EventContainerRestarted, cn, h, env, w, r)
}

type actionFn func(id string) (*freighterclient.Container, error)

func performControlAction(fn actionFn, tempState container.State, eventName controllers.EventName, cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// automatically expire all control action locks after 1 minute.
	// TODO: test with real ops and tweak this value
	err := cn.Lock(tempState, defaultLockTime, env.DB)
	if err != nil {
		if err == container.ErrLocked {
			return errContainerLocked
		}
		return err
	}
	defer cn.Unlock(env.DB)

	// broadcast transitioning state
	evtName, err := getEventName(tempState)
	if err != nil {
		return err
	}
	publishContainerEvent(env, evtName, cn)

	liveContainer, err := fn(cn.CID)
	if err != nil {
		return err
	}

	// reset the temp state, so freigther state will be exposed
	cn.TempState = sql.NullInt64{0, false}

	cn = hydrateContainer(cn, liveContainer, h)

	publishContainerEvent(env, eventName, cn)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
	}

	return nil
}

func destroy(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	// automatically expire destroy action lock after 1 minute.
	// TODO: test with real ops and tweak this value
	err := cn.Lock(container.StateTerminating, defaultLockTime, db)
	if err != nil {
		if err == container.ErrLocked {
			return errContainerLocked
		}
		return err
	}
	defer cn.Unlock(env.DB)

	// broadcast transitioning state
	publishContainerEvent(env, controllers.EventContainerDestroying, cn)

	cl, err := controllers.NewFreighterClient(db, h)
	if err != nil {
		return err
	}

	liveContainer, err := cl.Terminate(cn.CID)
	if err != nil {
		return err
	}

	// reset the temp state, so freigther state will be exposed
	cn.TempState = sql.NullInt64{0, false}

	err = cn.Delete(db)
	if err != nil {
		return err
	}

	cn = hydrateContainer(cn, liveContainer, h)

	publishContainerEvent(env, controllers.EventContainerDestroyed, cn)
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": serializers.SerializeContainer(cn),
	}
	return nil
}

func token(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	// do not issue token if container is locked
	locked, err := cn.IsLocked(db)
	if err != nil {
		return err
	}
	if locked {
		return errContainerLocked
	}

	r.ParseForm()
	permissions := r.PostForm["permission[]"]
	t, err := hmactoken.Generate(permissions, cn.Secret)
	if err != nil {
		return err
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"token": t,
	}
	return nil
}

func WithContainerAndHost(fn func(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error) middleware.Middleware {
	return func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		db := env.DB

		cn, err := container.Find(db, env.User, mux.Vars(r)["slug"])
		if err != nil {
			if err == sql.ErrNoRows {
				return errContainerNotFound
			}
			return err
		}

		h, err := host.Find(db, env.User, strconv.FormatInt(cn.HostID, 10))
		if err != nil {
			return err
		}
		if h == nil {
			return errHostNotFound
		}
		return fn(cn, h, env, w, r)
	}
}

type runningHost *host.Host
type hostFn func(h runningHost, env *middleware.Env, w http.ResponseWriter, r *http.Request) error

// requiresRunningHost is a wrapper which guarantees the action is called for a running host
func requiresRunningHost(fn hostFn) func(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	return func(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		if h.State != host.StateRunning {
			return controllers.ErrHostNotRunning
		}
		return fn(h, env, w, r)
	}
}

func hydrateContainer(c *container.Container, fc *freighterclient.Container, h *host.Host) *container.Container {
	c.SetState(fc.State)
	c.CID = fc.ID
	if c.Image == "" || !ignoredImageNames.MatchString(fc.Image) {
		c.Image = fc.Image
	}
	c.Slug = fc.Name
	c.IPAddress = fc.Host
	c.GatewayIP = fc.Gateway
	c.Cmd = fc.Cmd
	c.User = fc.User
	c.Entrypoint = fc.Entrypoint
	c.ExitCode = fc.ExitCode
	c.StartedAt = timePtr(fc.StartedAt)
	c.FinishedAt = timePtr(fc.FinishedAt)

	return c
}

func mkContainer(fc *freighterclient.Container, h *host.Host) *container.Container {
	c := &container.Container{
		CID:        fc.ID,
		Image:      fc.Image,
		Slug:       fc.Name,
		IPAddress:  fc.Host,
		GatewayIP:  fc.Gateway,
		UserID:     h.UserID,
		HostID:     h.ID,
		Cmd:        fc.Cmd,
		Entrypoint: fc.Entrypoint,
		ExitCode:   fc.ExitCode,
		User:       fc.User,
		StartedAt:  timePtr(fc.StartedAt),
		FinishedAt: timePtr(fc.FinishedAt),
	}
	c.SetState(fc.State)
	return c
}

func publishContainerEvent(env *middleware.Env, name controllers.EventName, c *container.Container) {
	env.Events.Publish(
		controllers.NewEvent(name, map[string]*serializers.Container{
			"container": serializers.SerializeContainer(c),
		}),
		env.User.Topic(),
	)
}

func prepareConfig(form url.Values, hv map[string]*volume.Volume) (*freighterclient.ContainerConfig, error) {
	c := &freighterclient.ContainerConfig{}
	c.Env = readParam(form, "env[]")
	vols := readParam(form, "volume[]")
	for _, v := range volumeSlugs(vols) {
		if _, ok := hv[v]; !ok {
			return nil, errVolumeNotFound
		}
	}
	c.Mounts = vols
	c.PortBindings = readParam(form, "port[]")
	if cmd := form.Get("cmd"); cmd != "" {
		args, err := shellwords.Parse(cmd)
		if err != nil {
			return nil, errInvalidCommand
		}
		c.Cmd = args
	}
	if priv, err := strconv.ParseBool(form.Get("privileged")); err == nil {
		c.Privileged = &priv
	}
	if pub, err := strconv.ParseBool(form.Get("publish_all")); err == nil {
		c.PublishAll = &pub
	}
	return c, nil
}

func readParam(form url.Values, name string) []string {
	vals, ok := form[name]
	switch {
	case !ok:
		return nil
	case len(vals) == 1 && vals[0] == "":
		// empty param
		return []string{}
	default:
		return vals
	}
}

func volumeSlugs(volParams []string) []string {
	slugs := []string{}
	for _, v := range volParams {
		slugs = append(slugs, strings.Split(v, ":")[0])
	}
	return slugs
}

func hostVolumes(d db.QueryReceiver, h *host.Host) (map[string]*volume.Volume, error) {
	slugsToVols := map[string]*volume.Volume{}
	hvols, err := volume.FindAll(d, h.ID)
	if err != nil {
		return nil, err
	}
	for _, hv := range hvols {
		slugsToVols[hv.Slug] = hv
	}
	return slugsToVols, nil
}

func timePtr(t time.Time) *time.Time {
	if !t.IsZero() {
		return &t
	}
	return nil
}

func expandFreighterClientError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "port is already allocated") {
		match := bindForMessage.FindStringSubmatch(err.Error())
		if len(match) >= 2 {
			return makeErrHostPortConflict(match[1])
		}
	}

	return err
}

func getEventName(s container.State) (controllers.EventName, error) {
	switch s {
	case container.StateStopping:
		return controllers.EventContainerStopping, nil
	case container.StateRestarting:
		return controllers.EventContainerRestarting, nil
	default:
		return "", fmt.Errorf("there's no associated event for the given state - %s", s)
	}
}
