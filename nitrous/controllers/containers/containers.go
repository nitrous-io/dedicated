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
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/pkg/shellwords"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/nitrous-common/hmactoken"
)

const transitionLockTime = 1 * time.Minute

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
	Commit  = WithContainerAndHost(commit, false)
	Config  = WithContainerAndHost(config, true)
	Show    = WithContainerAndHost(show, true)
	Start   = WithContainerAndHost(start, true)
	Stop    = WithContainerAndHost(stop, true)
	Restart = WithContainerAndHost(restart, true)
	Destroy = WithContainerAndHost(destroy, false)
	Token   = WithContainerAndHost(token, true)
)

var (
	PerformCreateFn = CreateContainer
)

type containerHandler func(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	var err error
	db := env.DB
	hSlug := r.FormValue("host")
	hosts := []*host.Host{}

	// If no host is provided, then get all hosts belonging to the user
	// and get all containers from all hosts
	if hSlug != "" {
		h, err := host.Find(db, env.User, hSlug)
		if err != nil {
			if err == sql.ErrNoRows {
				return errHostNotFound
			}
			return err
		}
		hosts = []*host.Host{h}
	} else {
		hosts, err = host.FindAllByUser(db, env.User.ID)
		if err != nil {
			return err
		}
	}

	containerHostMap := make(map[int64][]*container.Container)
	for _, host := range hosts {
		containers, err := container.FindAllByHostID(db, host.ID)
		if err != nil {
			env.Logger.Errorf("unable to find all containers: %v, for host %s", err, host.Slug)
			continue
		}
		containerHostMap[host.ID] = containers
	}

	env.Data = map[string]interface{}{}
	serContainers := listContainers(env, w, r, hosts, containerHostMap, false)
	env.Status = http.StatusOK
	env.Data["containers"] = serContainers
	return nil
}

func listContainers(env *middleware.Env, w http.ResponseWriter, r *http.Request, hosts []*host.Host, containerHostMap map[int64][]*container.Container, onlySaved bool) []*serializers.Container {
	db := env.DB

	containersResp := []*serializers.Container{}
	errorMessages := []string{}

	for _, h := range hosts {
		liveContainers, freighterErr := freighterList(db, h, env)

		savedContainers, ok := containerHostMap[h.ID]
		if !ok {
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
				cn = hydrateContainer(savedContainer, lc, h)
			} else {
				if onlySaved {
					continue
				}
				cn = mkContainer(lc, h)
			}

			containersResp = append(containersResp, serializers.SerializeContainer(cn))
		}
	}

	sort.Sort(serializers.BySlug(containersResp))
	if len(errorMessages) != 0 {
		env.Data["errors"] = errorMessages
	}

	return containersResp
}

type listresult struct {
	containers []*freighterclient.Container
	err        error
}

func freighterList(db db.QueryReceiver, h *host.Host, env *middleware.Env) ([]*freighterclient.Container, error) {
	cl, err := controllers.NewFreighterClient(db, h, env.UUID)
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
	cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
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
	cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
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
	cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
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

	sshURL, err := FindSSHURL(h, config)
	if err == nil {
		serialized.SSHURL = sshURL
	}

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

	cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
	if err != nil {
		return err
	}

	return doContainerExclusive(
		cn, env, container.StateStarting, transitionLockTime,
		func() error {
			// broadcast transitioning state
			PublishContainerEvent(env, controllers.EventContainerStarting, cn, nil)

			liveContainer, err := cl.Start(cn.CID, config)
			if err != nil {
				return expandFreighterClientError(err)
			}
			// reset the temp state, so freigther state will be exposed
			cn.TempState = sql.NullInt64{0, false}
			cn = hydrateContainer(cn, liveContainer, h)

			PublishContainerEvent(env, controllers.EventContainerStarted, cn, nil)
			env.Status = http.StatusOK
			env.Data = map[string]interface{}{
				"container": serializers.SerializeContainer(cn),
			}
			return nil
		})
}

type actionFn func(cl *controllers.FreighterClient, id string) (*freighterclient.Container, error)

func controlActionHandler(action actionFn, tempState container.State, eventName controllers.EventName) containerHandler {
	return func(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
		if err != nil {
			return err
		}

		return doContainerExclusive(
			cn, env, tempState, transitionLockTime,
			func() error {
				// broadcast transitioning state
				evtName, err := getEventName(tempState)
				if err != nil {
					return err
				}
				PublishContainerEvent(env, evtName, cn, nil)

				liveContainer, err := action(cl, cn.CID)
				if err != nil {
					return err
				}

				// reset the temp state, so freigther state will be exposed
				cn.TempState = sql.NullInt64{0, false}

				cn = hydrateContainer(cn, liveContainer, h)

				PublishContainerEvent(env, eventName, cn, nil)
				env.Status = http.StatusOK
				env.Data = map[string]interface{}{
					"container": serializers.SerializeContainer(cn),
				}

				return nil
			})
	}
}

var stop = controlActionHandler(
	func(cl *controllers.FreighterClient, id string) (*freighterclient.Container, error) {
		return cl.Stop(id)

	}, container.StateStopping, controllers.EventContainerStopped)

var restart = controlActionHandler(
	func(cl *controllers.FreighterClient, id string) (*freighterclient.Container, error) {
		return cl.Restart(id)

	}, container.StateRestarting, controllers.EventContainerRestarted)

func destroy(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	return doContainerExclusive(
		cn, env, container.StateTerminating, transitionLockTime,
		func() error {
			// broadcast transitioning state
			PublishContainerEvent(env, controllers.EventContainerDestroying, cn, nil)

			cl, err := controllers.NewFreighterClient(db, h, env.UUID)
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

			PublishContainerEvent(env, controllers.EventContainerDestroyed, cn, nil)
			env.Status = http.StatusOK
			env.Data = map[string]interface{}{
				"container": serializers.SerializeContainer(cn),
			}
			return nil
		})
}

func token(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// do not issue token if container is locked
	if cn.IsLocked() {
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

func WithContainerAndHost(fn containerHandler, allowCollab bool) middleware.Middleware {
	return func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		db := env.DB
		slug := mux.Vars(r)["slug"]

		cn, err := container.Find(db, env.User, slug)
		if err != nil {
			if err == sql.ErrNoRows {
				if !allowCollab {
					return errContainerNotFound
				}
				return withSharedContainerAndHost(fn, env, w, r)
			}
			return err
		}

		h, err := host.Find(db, env.User, strconv.FormatInt(cn.HostID, 10))
		if err != nil {
			if err == sql.ErrNoRows {
				return errHostNotFound
			}
			return err
		}
		return fn(cn, h, env, w, r)
	}
}

func withSharedContainerAndHost(fn containerHandler, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	slug := mux.Vars(r)["slug"]

	cn, err := container.FindShared(db, env.User, slug)
	if err != nil {
		if err == sql.ErrNoRows {
			return errContainerNotFound
		}
		return err
	}

	h, err := host.FindByIdentifier(db, strconv.FormatInt(cn.HostID, 10))
	if err != nil {
		if err == sql.ErrNoRows {
			return errHostNotFound
		}
		return err
	}
	return fn(cn, h, env, w, r)
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

func PublishContainerEvent(env *middleware.Env, name controllers.EventName, c *container.Container, add map[string]interface{}) {
	var userIDs []int64
	err := env.DB.Select(&userIDs, `SELECT user_id FROM container_collabs WHERE container_id = $1;`, c.ID)
	if err != nil {
		env.Logger.Errorf("Error retrieving collab user IDs for publishing container events, err: %v", err)
		return
	}
	userIDs = append(userIDs, c.UserID)

	data := map[string]interface{}{
		"container": serializers.SerializeContainer(c),
	}
	if env.User != nil {
		data["initiated_by"] = map[string]interface{}{
			"username": env.User.Username,
			"id":       env.User.ID,
		}
	}
	if add != nil {
		for k, v := range add {
			data[k] = v
		}
	}

	for _, userID := range userIDs {
		topic := pubsub.Topic(fmt.Sprintf("user-%d", userID))
		env.Events.Publish(controllers.NewEvent(name, data), topic)
	}
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
