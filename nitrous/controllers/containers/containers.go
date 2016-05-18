package containers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"

	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/billingprofile"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/containercollab"
	"github.com/nitrous-io/dedicated/nitrous/models/customdomain"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/image"
	"github.com/nitrous-io/dedicated/nitrous/models/usercontainerdata"
	"github.com/nitrous-io/dedicated/nitrous/models/volume"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/pkg/pubsub"
	"github.com/nitrous-io/dedicated/nitrous/pkg/shellwords"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/pkg/slugbuilder"
	"github.com/nitrous-io/nitrous-common/hmactoken"
)

const (
	transitionLockTime = 1 * time.Minute
	previewPortEnvVar  = "NITROUS_PREVIEW_PORT"
)

type WithContainerAndHostOpt int

// WithContainerAndHost options
const (
	AllowCollab WithContainerAndHostOpt = iota
	TouchHost
	BlockFreeUser
)

var (
	ignoredImageNames = regexp.MustCompile(`-reconfig-\d{19}\z`) // reconfig names include Unix epoch (nano) stamp
	bindForMessage    = regexp.MustCompile(`Bind\s+for\s+0.0.0.0:(\d+)`)
	// For parsing "scripts" params, e.g. "scripts[start]" = "rails s".
	scriptsParamsRe = regexp.MustCompile(`scripts\[([^\]]+)\]`)
)

var (
	errImageNameRequired = httperror.New(http.StatusBadRequest, "invalid_request", "image name is required")
	errHostNotFound      = middleware.ErrHostNotFound
	errVolumeNotFound    = httperror.New(http.StatusBadRequest, "invalid_state", "volume not found")
	errContainerNotFound = httperror.New(http.StatusNotFound, "container_not_found", "container does not exist")
	errInvalidPortspec   = httperror.New(http.StatusBadRequest, "invalid_request", "port spec invalid")
	errInvalidCommand    = httperror.New(http.StatusBadRequest, "invalid_request", "container cmd invalid")
	errContainerLocked   = httperror.New(http.StatusConflict, "container_locked", "container is locked")

	errFreeUserForbidden = httperror.New(http.StatusForbidden, "invalid_request", "only paid users have permission to access this section")

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
	Create  = middleware.HostViaForm("host", middleware.TouchHost(requiresRunningHost(create)))
	Commit  = WithContainerAndHost(commit, TouchHost)
	Config  = WithContainerAndHost(config, AllowCollab, TouchHost)
	Show    = WithContainerAndHost(show, AllowCollab)
	Start   = WithContainerAndHost(start, AllowCollab, TouchHost)
	Stop    = WithContainerAndHost(stop, AllowCollab, TouchHost)
	Restart = WithContainerAndHost(restart, AllowCollab, TouchHost)
	Destroy = WithContainerAndHost(destroy, TouchHost)
	Token   = WithContainerAndHost(ctoken, AllowCollab)
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
	certHostMap := make(map[int64]*cert.Cert)
	var hostIDs []int64
	for _, host := range hosts {
		hostIDs = append(hostIDs, host.ID)
	}

	containers, err := container.FindAllByHostIDs(db, hostIDs)
	if err != nil {
		logrus.Errorf("unable to find all containers: %v", err)
		return err
	}

	for _, container := range containers {
		containerHostMap[container.HostID] = append(containerHostMap[container.HostID], container)
	}

	certs, err := cert.FindAllByHostIDs(db, hostIDs)
	if err != nil {
		logrus.Errorf("unable to find all certs: %v", err)
		return err
	}

	for _, crt := range certs {
		certHostMap[crt.HostID] = crt
	}

	env.Data = map[string]interface{}{}
	serContainers := listContainers(env, w, r, hosts, containerHostMap, certHostMap, false)
	env.Status = http.StatusOK
	env.Data["containers"] = serContainers
	return nil
}

func listContainers(env *middleware.Env, w http.ResponseWriter, r *http.Request, hosts []*host.Host, containerHostMap map[int64][]*container.Container, certHostMap map[int64]*cert.Cert, onlySaved bool) []*serializers.Container {
	var (
		wg               sync.WaitGroup
		containersResp   = []*serializers.Container{}
		containersRespMu = &sync.Mutex{}
		errorMessages    = []string{}
		errorMessagesMu  = &sync.Mutex{}
	)

	for _, h := range hosts {
		wg.Add(1)
		go func(h *host.Host) {
			defer wg.Done()

			savedContainers, ok := containerHostMap[h.ID]
			if !ok {
				errorMessagesMu.Lock()
				errorMessages = append(errorMessages, fmt.Sprintf("Unable to locate containers for %s", h.Slug))
				errorMessagesMu.Unlock()
			}

			if h.State != host.StateRunning {
				for _, sc := range savedContainers {
					containersRespMu.Lock()
					containersResp = append(containersResp, serializers.SerializeContainer(sc))
					containersRespMu.Unlock()
				}

				return
			}

			crt, ok := certHostMap[h.ID]
			if !ok {
				errorMessagesMu.Lock()
				errorMessages = append(errorMessages, fmt.Sprintf("Unable to get cert for %s", h.Slug))
				errorMessagesMu.Unlock()
				return
			}

			liveContainers, err := freighterList(crt, h, env)
			if err != nil {
				// If we can't get through to the host, then pass them along without the Freighter specific
				// info to the frontend
				if err == controllers.ErrHostNotRunning || err == controllers.ErrHostIPUnknown {
					for _, sc := range savedContainers {
						containersRespMu.Lock()
						containersResp = append(containersResp, serializers.SerializeContainer(sc))
						containersRespMu.Unlock()
					}
				} else {
					errorMessagesMu.Lock()
					if strings.HasPrefix(err.Error(), "freighter:") {
						errorMessages = append(errorMessages, fmt.Sprintf("Unable to communicate with freighter for host %s", h.Slug))
					} else {
						logrus.Errorf("error listing containers for host %s, err: %v", h.Slug, err)
						errorMessages = append(errorMessages, fmt.Sprintf("Unable to locate containers for host %s", h.Slug))
					}
					errorMessagesMu.Unlock()
					return
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
					cn = hydrateContainer(env, savedContainer, lc, h)
				} else {
					if onlySaved {
						continue
					}
					cn = mkContainer(lc, h)
				}

				containersRespMu.Lock()
				containersResp = append(containersResp, serializers.SerializeContainer(cn))
				containersRespMu.Unlock()
			}

			for _, sc := range savedContainers {
				if sc.State == container.StateCreating {
					containersRespMu.Lock()
					containersResp = append(containersResp, serializers.SerializeContainer(sc))
					containersRespMu.Unlock()
				}
			}
		}(h)
	}
	wg.Wait()

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

func freighterList(crt *cert.Cert, h *host.Host, env *middleware.Env) ([]*freighterclient.Container, error) {
	cl, err := controllers.NewFreighterClientWithCert(crt, h, env.UUID)
	if err != nil {
		return nil, err
	}

	liveContainers, err := cl.List()
	if err != nil {
		return nil, err
	}
	return liveContainers, nil
}

func create(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
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

	slug := slugbuilder.Slugify(name)
	repo := fmt.Sprintf("%s:__snapshot__%s", slug, time.Now().Format("20060102-150405"))

	created, err := cl.Commit(cn.CID, repo)
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
		UserID:        env.User.ID,
		HostID:        h.ID,
		FreighterID:   created.ID,
		Name:          name,
		Slug:          created.Name,
		Tag:           created.Tag,
		OriginalImage: sql.NullString{cn.Image, true},
		IsSnapshot:    true,
		ActualSize:    created.ActualSize,
		VirtualSize:   created.VirtualSize,
		BuiltAt:       builtAt,
	}
	if err := img.InsertOrUpdate(env.DB); err != nil { // log and continue
		logrus.Errorf("containers.create: failed to insert or update image '%+v'", img)
	}

	sImg := serializers.SerializeImage(img)
	liveContainer, err := cl.Info(cn.CID)
	if err != nil {
		return err
	}
	cn = hydrateContainer(env, cn, liveContainer, h)

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
		if err := deleteIfNotFound(env.DB, cn, err); err != nil {
			return err
		}
		return err
	}
	cn = hydrateContainer(env, cn, liveContainer, h)

	config, err := cl.Config(cn.CID)
	if err != nil {
		return err
	}

	serialized := serializers.SerializeContainer(cn)
	serialized.Config = serializers.SerializeContainerConfig(config)

	var wg sync.WaitGroup
	var sshURL string
	userData := map[string]string{}

	wg.Add(1)
	go func() {
		defer wg.Done()

		url, err := FindSSHURL(h, config)
		if err == nil {
			sshURL = url
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		ucd, err := usercontainerdata.Find(env.DB, env.User.ID, cn.ID)
		if err != nil {
			if err == sql.ErrNoRows {
				return
			}
			logrus.Errorf("Error retrieving usercontainerdata err: %v", err)
			return
		}

		for k, v := range ucd.Metadata.Map {
			if v.Valid {
				userData[k] = v.String
			}
		}
	}()

	wg.Wait()

	serialized.SSHURL = sshURL
	serialized.UserData = userData

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"container": serialized,
	}

	return nil
}

func start(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	config := &freighterclient.ContainerConfig{}
	hv, err := hostVolumes(env.DB, h)
	if err != nil {
		return err
	}
	r.ParseForm()

	if err := prepareConfig(config, r.PostForm, hv); err != nil {
		return err
	}

	cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
	if err != nil {
		return err
	}

	return doContainerExclusive(
		cn, env, container.StateStarting, transitionLockTime,
		func() error {
			logger := logrus.WithFields(logrus.Fields{
				"uuid":           env.UUID,
				"container_slug": cn.Slug,
			})

			// Publish transitioning state
			PublishContainerEvent(env, controllers.EventContainerStarting, cn, nil)

			liveContainer, err := cl.Start(cn.CID, config)
			if err != nil {
				return expandFreighterClientError(env.DB, cn, err)
			}

			// Update port mappings
			portMappings, err := GetPortMappingsJsonFrom(liveContainer)
			if err == nil {
				if err := cn.UpdateColumn(env.DB, "port_mappings", portMappings); err != nil {
					logger.Errorf("container.start: failed to update port mappings: %v", err)
				}
			} else {
				logger.Errorf("container.start: Failed to get preview port for container(%+v) : %+v", liveContainer, err)
			}

			cn = hydrateContainer(env, cn, liveContainer, h)
			cn = syncState(env, cn, liveContainer)

			PublishContainerEvent(env, controllers.EventContainerStarted, cn, nil)
			env.Status = http.StatusOK
			env.Data = map[string]interface{}{
				"container": serializers.SerializeContainer(cn),
			}
			return nil
		})
}

type actionFn func(env *middleware.Env, cl *controllers.FreighterClient, cn *container.Container) (*freighterclient.Container, error)

func controlActionHandler(action actionFn, state string, transitionalEventName, eventName controllers.EventName) containerHandler {
	return func(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
		if err != nil {
			return err
		}

		return doContainerExclusive(
			cn, env, state, transitionLockTime,
			func() error {
				PublishContainerEvent(env, transitionalEventName, cn, nil)

				liveContainer, err := action(env, cl, cn)
				if err != nil {
					if err := deleteIfNotFound(env.DB, cn, err); err != nil {
						return err
					}
					return err
				}

				cn = hydrateContainer(env, cn, liveContainer, h)
				cn = syncState(env, cn, liveContainer)

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
	func(env *middleware.Env, cl *controllers.FreighterClient, cn *container.Container) (*freighterclient.Container, error) {
		return cl.Stop(cn.CID)
	}, container.StateStopping,
	controllers.EventContainerStopping,
	controllers.EventContainerStopped)

var restart = controlActionHandler(
	func(env *middleware.Env, cl *controllers.FreighterClient, cn *container.Container) (*freighterclient.Container, error) {
		liveContainer, err := cl.Restart(cn.CID)
		if err != nil {
			return nil, err
		}

		logger := logrus.WithFields(logrus.Fields{
			"uuid":           env.UUID,
			"container_slug": cn.Slug,
		})

		// Update port mappings
		portMappings, err := GetPortMappingsJsonFrom(liveContainer)
		if err != nil {
			logger.Errorf("container.restart: Failed to get preview port for container(%+v) : %+v", liveContainer, err)
			return nil, err
		}

		if err := cn.UpdateColumn(env.DB, "port_mappings", portMappings); err != nil {
			logger.Errorf("container.restart: failed to update preview port: %v", err)
			return nil, err
		}

		return liveContainer, nil

	}, container.StateRestarting,
	controllers.EventContainerRestarting,
	controllers.EventContainerRestarted)

func destroy(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	return doContainerExclusive(
		cn, env, container.StateTerminating, transitionLockTime,
		func() error {
			logger := logrus.WithFields(logrus.Fields{
				"uuid":           env.UUID,
				"container_slug": cn.Slug,
			})

			PublishContainerEvent(env, controllers.EventContainerDestroying, cn, nil)

			cl, err := controllers.NewFreighterClient(db, h, env.UUID)
			if err != nil {
				return err
			}

			liveContainer, err := cl.Terminate(cn.CID)
			if err != nil {
				if err := deleteIfNotFound(db, cn, err); err != nil {
					return err
				}
				return err
			}

			cn.State = container.StateTerminated
			if err := cn.Delete(db); err != nil {
				return err
			}

			// Delete the image the container is using, if it is a Nitrous
			// template.
			if image.IsNitrousTemplate(cn.Image) {
				// Delete image from host.
				if err := cl.RemoveImage(cn.Image); err != nil {
					logger.Errorf("error removing Nitrous image after removing container, err: %v", err)
				} else {
					// Delete image from database if it exists.
					if img, err := image.Find(db, env.User, h, cn.Image); err == nil {
						if err := img.Delete(db); err != nil {
							logger.Errorf("failed to delete image from database, err: %v", err)
						}
					}
				}
			}

			// Delete the custom domains that is associate to the container
			if err := customdomain.Destroy(db, cn.ID, -1); err != nil {
				logger.Errorf("failed to delete custom domains from database, err: %v", err)
			}

			cn = hydrateContainer(env, cn, liveContainer, h)

			PublishContainerEvent(env, controllers.EventContainerDestroyed, cn, nil)
			env.Status = http.StatusOK
			env.Data = map[string]interface{}{
				"container": serializers.SerializeContainer(cn),
			}
			return nil
		})
}

func ctoken(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
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

func WithContainerAndHost(fn containerHandler, opts ...WithContainerAndHostOpt) middleware.Middleware {
	allowCollab, touchHost, blockFreeUser := false, false, false
	for _, opt := range opts {
		switch opt {
		case AllowCollab:
			allowCollab = true
		case TouchHost:
			touchHost = true
		case BlockFreeUser:
			blockFreeUser = true
		}
	}
	return func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		db := env.DB
		slug := mux.Vars(r)["slug"]

		cn, err := container.FindShared(db, env.User, slug)
		if err != nil {
			if err == sql.ErrNoRows {
				return errContainerNotFound
			}
			return err
		}

		shared := false
		if cn.UserID != env.User.ID {
			if allowCollab {
				shared = true
			} else {
				return errContainerNotFound
			}
		}

		h, err := host.FindByIdentifier(db, strconv.FormatInt(cn.HostID, 10))
		if err != nil {
			if err == sql.ErrNoRows {
				return errHostNotFound
			}
			return err
		}
		h.Shared = shared
		if touchHost {
			touchFn := func() {
				if err := h.Touch(db); err != nil {
					// log and passthrough
					logrus.WithFields(logrus.Fields{
						"host_id": h.ID,
						"slug":    h.Slug,
					}).Errorf("containers: error touching host: %v", err)
				}
			}
			touchFn()
			beforeOp := time.Now()
			defer func() {
				// if the operation takes too long, touch host again at the end
				if time.Since(beforeOp) > 1*time.Minute {
					touchFn()
				}
			}()
		}

		if blockFreeUser {
			var bp *billingprofile.BillingProfile
			if h.OrgID.Valid {
				bp, err = billingprofile.FindForOrg(db, h.OrgID.Int64)
				if err != nil {
					return err
				}
			} else {
				bp, err = billingprofile.Find(db, cn.UserID)
				if err != nil {
					return err
				}
			}
			if bp.IsFreeTier() {
				return errFreeUserForbidden
			}
		}

		return fn(cn, h, env, w, r)
	}
}

type hostFn func(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error

// requiresRunningHost is a wrapper which guarantees the action is called for a running host
func requiresRunningHost(fn hostFn) func(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	return func(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		if h.State != host.StateRunning {
			return controllers.ErrHostNotRunning
		}
		return fn(h, env, w, r)
	}
}

func hydrateContainer(env *middleware.Env, c *container.Container, fc *freighterclient.Container, h *host.Host) *container.Container {
	// always sync state from freighter to state container model
	if c.State != fc.State &&
		(fc.State == container.StateRunning || fc.State == container.StateStopped) &&
		!c.IsLocked() {
		c.State = fc.State
	}

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
		State:      fc.State,
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
	return c
}

func syncState(env *middleware.Env, c *container.Container, fc *freighterclient.Container) *container.Container {
	if c.State != fc.State &&
		(fc.State == container.StateRunning || fc.State == container.StateStopped) {
		c.State = fc.State
		if err := c.UpdateColumn(env.DB, "state", fc.State); err != nil {
			logrus.Errorf("container.action: failed to update state to %v: %v", fc.State, err)
		}
	}

	return c
}

func PublishContainerEvent(env *middleware.Env, name controllers.EventName, c *container.Container, additionalData map[string]interface{}) {
	userIDs, err := containercollab.GetCollaboratorIDs(env.DB, c.ID)
	if err != nil {
		logrus.Errorf("Error retrieving collab user IDs for publishing container events, err: %v", err)
		return
	}
	userIDs = append(userIDs, c.UserID)
	data := map[string]interface{}{}

	if env.User != nil {
		data["initiated_by"] = map[string]interface{}{
			"id":       env.User.ID,
			"username": env.User.Username,
		}
	}

	for k, v := range additionalData {
		data[k] = v
	}

	PublishContainerEventToUsers(env.Events, name, c, userIDs, data)
}

func PublishContainerEventToUsers(events *pubsub.Pubsub, name controllers.EventName, c *container.Container, userIDs []int64, additionalData map[string]interface{}) {
	data := map[string]interface{}{
		"container": serializers.SerializeContainer(c),
	}

	for k, v := range additionalData {
		data[k] = v
	}

	for _, userID := range userIDs {
		topic := pubsub.Topic(fmt.Sprintf("user-%d", userID))
		events.Publish(controllers.NewEvent(name, data), topic)
	}
}

// prepareConfig accepts a ContainerConfig and modifies it with user-specified settings
func prepareConfig(c *freighterclient.ContainerConfig, form url.Values, hv map[string]*volume.Volume) error {
	c.Env = append(c.Env, readParam(form, "env[]")...)
	c.PortBindings = append(c.PortBindings, readParam(form, "port[]")...)

	volParams := readParam(form, "volume[]")
	var vols []string
	if len(volParams) > 0 {
		for _, v := range volParams {
			if v == "/opt/nitrous:/.nitrous:ro" {
				// this is occasionally present in user containers and results in a
				// 'Volume not found' error when trying to reconfig.  We expect
				// this to be automatically added on the freighter end if
				// necessary, so excluding this should fix the bug with no unwanted
				// effects.
				continue
			}
			vols = append(vols, v)
		}
	}
	for _, v := range volumeSlugs(vols) {
		if _, ok := hv[v]; !ok {
			return errVolumeNotFound
		}
	}
	c.Mounts = vols

	if cmd := form.Get("cmd"); cmd != "" {
		args, err := shellwords.Parse(cmd)
		if err != nil {
			return errInvalidCommand
		}
		c.Cmd = args
	}

	if priv, err := strconv.ParseBool(form.Get("privileged")); err == nil {
		c.Privileged = &priv
	}

	if pub, err := strconv.ParseBool(form.Get("publish_all")); err == nil {
		c.PublishAll = &pub
	}

	scripts := map[string]string{}
	for k, _ := range form {
		match := scriptsParamsRe.FindStringSubmatch(k)
		if len(match) == 2 {
			scriptKey := match[1]
			scripts[scriptKey] = form.Get(k)
		}
	}
	c.Scripts = scripts

	return nil
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

func expandFreighterClientError(d db.QueryReceiver, cn *container.Container, err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "port is already allocated") {
		match := bindForMessage.FindStringSubmatch(err.Error())
		if len(match) >= 2 {
			return makeErrHostPortConflict(match[1])
		}
	}

	if err := deleteIfNotFound(d, cn, err); err != nil {
		return err
	}
	return err
}

// given a container and an error from freighterclient, determines if the error
// means that the container was not found in Docker and delete if so
func deleteIfNotFound(d db.QueryReceiver, cn *container.Container, fcErr error) error {
	if !strings.Contains(fcErr.Error(), "container not found") {
		return nil
	}
	logrus.WithFields(logrus.Fields{
		"container": cn.ID,
		"host":      cn.HostID,
	}).Errorf("container is marked as terminated in workspace")
	cn.State = container.StateTerminated
	if err := cn.Delete(d); err != nil {
		logrus.Errorf("unable to mark the container as terminated in db: %v", err)
	}
	return errContainerNotFound
}

func GetPortMappingsJsonFrom(container *freighterclient.Container) (string, error) {
	var (
		previewPort string
		err         error
	)

	portMappings := make(map[string]string)

	// Get preview port from environments
	for _, val := range container.Env {
		split := strings.SplitN(val, "=", 2)
		if len(split) == 2 && split[0] == previewPortEnvVar {
			previewPort = split[1]
		}
	}

	// Get mapping port
	for incomingPort, mappingPorts := range container.Ports {
		if incomingPort == previewPort {
			if len(mappingPorts) > 0 {
				portMappings["preview"] = mappingPorts[0]
			}
		}
		if len(mappingPorts) > 0 {
			portMappings[incomingPort] = mappingPorts[0]
		}
	}

	portMappingsJson, err := json.Marshal(portMappings)
	if err != nil {
		return "", err
	}

	return string(portMappingsJson), nil
}
