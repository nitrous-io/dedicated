package hosts

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/respec"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	errSizeRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "size is required")
	errNameRequired             = httperror.New(http.StatusBadRequest, "invalid_request", "name is required")
	errRegionRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "region is required")
	errPlanNotFound             = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found for the given size")
	errRegionNotFound           = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	errProvidertemplateNotFound = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and regions combination")
	errHostStartInvalidState    = httperror.New(http.StatusBadRequest, "invalid_state", "workspace is not stopped")
	errHostStopInvalidState     = httperror.New(http.StatusBadRequest, "invalid_state", "workspace is not running")
	errHostRestartInvalidState  = httperror.New(http.StatusBadRequest, "invalid_state", "workspace is not running")
	errHostNotRunning           = httperror.New(http.StatusBadRequest, "invalid_state", "workspace is not running")
	errHostCreationNotAllowed   = httperror.New(http.StatusForbidden, "request_not_allowed", "not allowed to create more hosts")
	errMultipleRequests         = httperror.New(http.StatusConflict, "workspace_locked", "another workspace creation request is currently being processed")
	errHostLocked               = httperror.New(http.StatusConflict, "host_locked", "host is locked for another request")
	errHostSlugTaken            = httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken")
	errHostNoPendingRespec      = httperror.New(http.StatusBadRequest, "invalid_request", "workspace has no upgrades available")
	errRegionDisabled           = httperror.New(http.StatusBadRequest, "region_disabled", "region is disabled")
	errBasicAuthFailed          = httperror.New(http.StatusUnauthorized, "unauthorized", "")
	errStoppingNotAllowed       = httperror.New(http.StatusForbidden, "request_not_allowed", "users in the free tier cannot stop a host")
	errRestartingNotAllowed     = httperror.New(http.StatusForbidden, "request_not_allowed", "users in the free tier cannot restart a host")
	errFreeTimeLimitExceeded    = httperror.New(http.StatusForbidden, "free_time_limit_exceeded", "You have exceeded your daily usage")
	errUpgradeHostFailed        = httperror.New(http.StatusInternalServerError, "internal_server_error", "an error occurred while attempting to update your workspace")
)

var HostbuilderClient = provisioner.HostbuilderClient
var defaultProvider string
var aesKey string

func init() {
	aesKey = envvars.AESKey
	if aesKey == "" {
		panic("AES_KEY is not defined")
	}

	defaultProvider = envvars.DefaultProvider
	if defaultProvider == "" {
		panic("DEFAULT_PROVIDER is not set")
	}
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hosts, err := host.FindAllByUser(env.DB, env.User.ID)
	if err != nil {
		return err
	}

	hostsResp := make([]*serializers.Host, len(hosts))
	for i, h := range hosts {
		hostsResp[i] = serializers.SerializeHostWithUser(h, env.User)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hosts": hostsResp,
	}
	return nil
}

func Refresh(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	env.Events.Publish(
		controllers.NewEvent(controllers.EventHostsRefreshed, map[string]interface{}{
		// no data necessary -- event used to trigger reload of all hosts
		}),
		env.User.Topic(),
	)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{}
	return nil
}

func Show(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	var ver *freighterclient.HostVersions

	if cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID); err == nil {
		if ver, err = cl.HostVersions(); err == nil {
			if h.DockerVersion != "" && ver.DockerVersion != h.DockerVersion {
				h.UpdateAvailable = host.UpdateAvailableHard
			} else if h.FreighterdVersion != "" && ver.FreighterdVersion != h.FreighterdVersion {
				h.UpdateAvailable = host.UpdateAvailableSoft
			}
			if env.User.Admin {
				h.LiveDockerVersion = ver.DockerVersion
				h.LiveFreighterdVersion = ver.FreighterdVersion
			}
		}
	}

	if !envvars.EnableSelfUpgrade && !env.User.Admin {
		h.UpdateAvailable = ""
	}

	if _, err := respec.Find(env.DB, h.ID); err == nil {
		h.PendingRespec = true
	}

	serializeHost(env, h)
	return nil
}

func Update(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// since name can be null, we interpret absence of name field to mean no
	// change, and presence of name field with empty string to mean set it to
	// null
	r.ParseForm()
	if _, ok := r.PostForm["name"]; !ok {
		serializeHost(env, h)
		return nil
	}

	name := r.PostFormValue("name")
	h.Name.String, h.Name.Valid = name, name != ""
	if err := h.Update(env.DB); err != nil {
		return err
	}

	PublishHostEvent(env, controllers.EventHostUpdated, h)
	serializeHost(env, h)
	return nil
}

func UpdateVersions(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if !env.User.Admin {
		return httperror.New(http.StatusNotFound, "not_found", "not found")
	}

	fv := r.FormValue("freighterd_version")
	dv := r.FormValue("docker_version")

	err := h.UpdateVersions(env.DB, fv, dv)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	serializeHost(env, h)
	return nil
}

var defaultPeriod = 100 * time.Millisecond

func Stats(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// Host should be running and have an IP address.
	if h.State != host.StateRunning || h.IPAddress.String == "" {
		return errHostNotRunning
	}

	cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
	if err != nil {
		return err
	}

	period, err := time.ParseDuration(r.FormValue("period"))
	if err != nil || period == 0 {
		period = defaultPeriod
	}

	info, err := cl.HostInfo(period)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"stats": serializers.SerializeHostInfo(info),
	}
	return nil
}

func Destroy(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if h.State == host.StateTerminating {
		return errHostLocked
	}

	db := env.DB

	// Update state to terminating to lock and make it visible to users
	if err := transitionHostState(db, h, host.StateTerminating, env.User.ID); err != nil {
		return err
	}

	err := db.WithTransaction(func(tx *sqlx.Tx) error {
		return hostcontrol.Terminate(tx, env.Logger, h, hostbuilderHeaders(env), env.User.ID)
	})
	if err != nil {
		return err
	}

	if env.User.Admin && r.FormValue("reassign") == "true" {
		_, err = db.Exec("UPDATE hosts SET user_id = $1 WHERE id = $2;", env.User.ID, h.ID)
		if err != nil {
			env.Logger.Errorf("Error reassigning host, err: %+v", err)
		}
	}

	trackWorkspaces(env, h.UserID)
	PublishHostEvent(env, controllers.EventHostDestroyed, h)
	serializeHost(env, h)
	return nil
}

func Start(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if h.State != host.StateStopped {
		return errHostStartInvalidState
	}

	db := env.DB
	logger := env.Logger

	err := freetier.StartHostAllowed(db, env.User, h)
	if err != nil {
		switch err {
		case freetier.ErrFreeTimeLimitExceeded:
			return errFreeTimeLimitExceeded
		default:
			return err
		}
	}

	origState := h.State
	if err := transitionHostState(db, h, host.StateStarting, env.User.ID); err != nil {
		return err
	}

	PublishHostEvent(env, controllers.EventHostStarting, h)

	urlArgs := host.URLArgsFromHost(h)
	info, err := HostbuilderClient.Start(h.Provider, h.MachineConfig(), hostbuilderHeaders(env), urlArgs)
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			env.Logger.Errorf("hostbuilder timeout starting host: %v", e)
		}

		// Rollback to previous state, log any errors while rolling back, but
		// return original hostbuilderclient error.
		if err := transitionHostState(db, h, origState, env.User.ID); err != nil {
			env.Logger.Errorf("hosts.Start: Unable to restore host state: %v", err)
		}

		PublishHostEvent(env, controllers.EventHostUpdated, h)
		return err
	}

	// Start successful
	err = db.WithTransaction(func(tx *sqlx.Tx) error {
		h.State = host.StateRunning
		h.IPAddress = sql.NullString{info.IPAddress, true}
		h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}

		if err := h.Update(tx); err != nil {
			return err
		}

		return hosttransition.Record(tx, h, host.StateStarting, *h.UpdatedAt, env.User.ID)
	})
	if err != nil {
		logger.Errorf("hosts.Start: failed to update new host state: %v", err)
		return err
	}

	PublishHostEvent(env, controllers.EventHostStarted, h)
	serializeHost(env, h)
	return nil
}

func Stop(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	canStop, err := freetier.StopHostAllowed(env.DB, env.User)

	if err != nil {
		return err
	}

	if !canStop {
		return errStoppingNotAllowed
	}

	force, _ := strconv.ParseBool(r.FormValue("force"))
	if !force && h.State != host.StateRunning {
		return errHostStopInvalidState
	}

	cns, err := container.FindAllByHostID(env.DB, h.ID)
	if err != nil {
		return err
	}

	if !force {
		slugs := getTransientContainerSlugs(cns)
		if slugs != "" {
			return httperror.New(http.StatusBadRequest, "invalid_container_state", fmt.Sprintf("containers(%s) are not stopped", slugs))
		}
	}

	evHandler := func(ev hostcontrol.EventType) {
		if ev == hostcontrol.EventHostStopping {
			PublishHostEvent(env, controllers.EventHostStopping, h)
		}
		if ev == hostcontrol.EventHostUpdated {
			PublishHostEvent(env, controllers.EventHostUpdated, h)
		}
	}

	// locking done inside hostcontrol.Stop
	err = hostcontrol.Stop(env.DB, env.Logger, h, hostbuilderHeaders(env), evHandler, env.User.ID)
	if err != nil {
		if err == host.ErrUpdateFailed {
			return errHostLocked
		}

		return err
	}

	for _, cn := range cns {
		state := container.State(cn.TempState.Int64)
		if !cn.TempState.Valid || (cn.TempState.Valid && state != container.StateStopping) {
			PublishContainerStopped(env, cn)
		}
	}
	PublishHostEvent(env, controllers.EventHostStopped, h)
	serializeHost(env, h)
	return nil
}

func Restart(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	canStop, err := freetier.StopHostAllowed(db, env.User)
	if err != nil {
		return err
	}
	if !canStop {
		return errRestartingNotAllowed
	}

	force, _ := strconv.ParseBool(r.FormValue("force"))
	if !force && h.State != host.StateRunning {
		return errHostRestartInvalidState
	}

	cns, err := container.FindAllByHostID(db, h.ID)
	if err != nil {
		return err
	}

	if !force {
		slugs := getTransientContainerSlugs(cns)
		if slugs != "" {
			return httperror.New(http.StatusBadRequest, "invalid_container_state", fmt.Sprintf("containers(%s) are not stopped", slugs))
		}
	}

	origState := h.State
	if err := transitionHostState(db, h, host.StateRestarting, env.User.ID); err != nil {
		return err
	}

	PublishHostEvent(env, controllers.EventHostRestarting, h)

	urlArgs := host.URLArgsFromHost(h)
	info, err := HostbuilderClient.Restart(h.Provider, h.MachineConfig(), hostbuilderHeaders(env), urlArgs)
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			env.Logger.Errorf("hostbuilder timeout restarting host: %v", e)
		}

		// Rollback to previous state, log any errors while rolling back, but
		// return original hostbuilderclient error.
		if err := transitionHostState(db, h, origState, env.User.ID); err != nil {
			env.Logger.Errorf("hosts.Restart: Unable to restore host state: %v", err)
		}

		PublishHostEvent(env, controllers.EventHostUpdated, h)
		return err
	}

	// Success restart
	err = db.WithTransaction(func(tx *sqlx.Tx) error {
		h.State = host.StateRunning
		h.IPAddress = sql.NullString{info.IPAddress, true}
		h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}

		if err := h.Update(tx); err != nil {
			return err
		}

		return hosttransition.Record(tx, h, host.StateRestarting, *h.UpdatedAt, env.User.ID)
	})
	if err != nil {
		env.Logger.Errorf("hosts.Restart: failed to update to new host state: %v", err)
		return err
	}

	for _, cn := range cns {
		state := container.State(cn.TempState.Int64)
		if !cn.TempState.Valid || (cn.TempState.Valid && state != container.StateStopping) {
			PublishContainerStopped(env, cn)
		}
	}

	PublishHostEvent(env, controllers.EventHostRestarted, h)
	serializeHost(env, h)
	return nil
}

func Respec(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// FIXME: should not allow concurrent request with other transitions, i.e. updating
	if h.State == host.StateRespeccing {
		return errHostLocked
	}

	db := env.DB

	rs, err := respec.Find(db, h.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			return errHostNoPendingRespec
		}
		return err
	}

	pt, err := providertemplate.FindByID(db, rs.ProviderTemplateID)
	if err != nil {
		return err
	}

	err = db.WithTransaction(func(tx *sqlx.Tx) error {
		if err := h.UpdateState(db, host.StateRespeccing); err != nil {
			if err == host.ErrUpdateFailed {
				return errHostLocked
			}
			return err
		}

		return rs.UpdateState(db, respec.StateProcessing)
	})
	if err != nil {
		return err
	}

	cfg := h.MachineConfig()
	for k, v := range pt.Config.Map {
		if v.Valid {
			cfg[k] = v.String
		} else {
			cfg[k] = ""
		}
	}

	// Apply provider-specific configuration.
	prov := env.Provisioner
	cfg, err = prov.ApplyProviderConfig(cfg, pt, h)
	if err != nil {
		env.Logger.Errorf("failed to apply provider-specific config, err: %v", err)
		return err
	}

	go func() {
		info, err := HostbuilderClient.Respec(h.Provider, cfg, hostbuilderHeaders(env), host.URLArgsFromHost(h))
		if err != nil {
			if err := h.UpdateState(db, host.StateRespecFailed); err != nil {
				env.Logger.Errorf("error updating host after respec failed: %v", err)
			}
			if err := rs.UpdateState(db, respec.StateFailed); err != nil {
				env.Logger.Errorf("error updating respec state to failed: %v", err)
			}

			env.Events.Publish(
				controllers.NewEvent(controllers.EventHostRespecFailed, map[string]interface{}{
					"host":  serializers.SerializeHost(h),
					"error": err.Error(),
				}),
				env.User.Topic(),
			)
			return
		}

		h.ProviderTemplateID = rs.ProviderTemplateID
		h.State = host.StateRunning
		h.IPAddress = sql.NullString{info.IPAddress, true}
		if err := h.UpdateAfterRespec(db); err != nil {
			env.Logger.Errorf("error updating host after respec completed: %v", err)
		}
		if err := rs.UpdateState(db, respec.StateCompleted); err != nil {
			env.Logger.Errorf("error updating respec state to completed: %v", err)
		}

		PublishHostEvent(env, controllers.EventHostRespecced, h)
	}()

	PublishHostEvent(env, controllers.EventHostRespeccing, h)
	env.Status = http.StatusAccepted
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}

	return nil
}

func UpdateSoftware(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// FIXME: should not allow concurrent request with other transitions, i.e. respeccing
	if h.State == host.StateUpdating {
		return errHostLocked
	}

	db := env.DB

	cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
	if err != nil {
		return err
	}

	origState := h.State
	if err := h.UpdateState(db, host.StateUpdating); err != nil {
		if err == host.ErrUpdateFailed {
			return errHostLocked
		}
		return err
	}

	err = cl.UpgradeServer(&freighterclient.HostVersions{
		DockerVersion:     h.DockerVersion,
		FreighterdVersion: h.FreighterdVersion,
	})
	if err != nil {
		if err := h.UpdateState(db, origState); err != nil {
			env.Logger.Errorf("failed to restore host state failed to upgrade, err: %v", err)
		}

		if err == freighterclient.ErrAlreadyUpgraded {
			PublishHostEvent(env, controllers.EventHostSoftwareUpdated, h)
			serializeHost(env, h)
			return nil
		}

		env.Logger.Errorf("failed to update server, err: %v", err)
		return errUpgradeHostFailed
	}

	PublishHostEvent(env, controllers.EventHostSoftwareUpdating, h)
	env.Status = http.StatusAccepted
	env.Data = map[string]interface{}{
		"status": "accepted",
	}
	return nil
}

func AptAllowed(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	authHeader := r.Header.Get("Authorization")
	prefix := "Basic "
	if len(authHeader) <= len(prefix) ||
		(!strings.HasPrefix(authHeader, prefix) &&
			!strings.HasPrefix(authHeader, strings.ToLower(prefix))) {
		requireAuth(w)
		return errBasicAuthFailed
	}

	s := authHeader[len(prefix):]
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		requireAuth(w)
		env.Logger.Warnf("cannot decode Basic Auth header, err: %v", err)
		return errBasicAuthFailed
	}
	pair := strings.SplitN(string(b), ":", 2)
	if len(pair) != 2 {
		requireAuth(w)
		return errBasicAuthFailed
	}

	hostID, err := strconv.ParseInt(pair[0], 10, 64)
	if err != nil {
		requireAuth(w)
		env.Logger.Warnf("cannot decode parse host ID from %v, err: %v", pair[0], err)
		return errBasicAuthFailed
	}
	password := pair[1]

	hostSlug := mux.Vars(r)["slug"]
	h, err := host.FindByIdentifier(env.DB, hostSlug)
	if err != nil {
		requireAuth(w)
		return errBasicAuthFailed
	}
	if hostID != h.ID || password != h.Secret {
		requireAuth(w)
		return errBasicAuthFailed
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}

	return nil
}

func requireAuth(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="nitrous"`)
}

func hostbuilderHeaders(env *middleware.Env) http.Header {
	h := http.Header{}
	h.Set(constants.NitrousUUIDHeader, env.UUID)
	return h
}

func serializeHost(env *middleware.Env, h *host.Host) {
	u := env.User
	if u.ID != h.UserID {
		var err error
		u, err = user.FindByID(env.DB, h.UserID)
		if err != nil {
			env.Logger.Errorf("Error retrieving owner %d for host %d, err: %+v", h.UserID, h.ID, err)
		}
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHostWithUser(h, u),
	}
}

func PublishHostEvent(env *middleware.Env, name controllers.EventName, h *host.Host) {
	u := env.User
	if u.ID != h.UserID {
		var err error
		u, err = user.FindByID(env.DB, h.UserID)
		if err != nil {
			env.Logger.Errorf("Error retrieving owner %d for host %d, err: %+v", h.UserID, h.ID, err)
		}
	}
	env.Events.Publish(
		controllers.NewEvent(name, map[string]*serializers.Host{
			"host": serializers.SerializeHostWithUser(h, u),
		}),
		env.User.Topic(),
	)
}

func PublishContainerstopped(env *middleware.Env, h *host.Host) {
	cns, err := container.FindAllByHostID(env.DB, h.ID)
	if err != nil {
		env.Logger.Errorf("hosts.Stop: Unable to list host containers: %v", err)
		return
	}
	for _, cn := range cns {
		PublishContainerStopped(env, cn)
	}
}

func PublishContainerStopped(env *middleware.Env, cn *container.Container) {
	cn.State = container.StateStopped
	containers.PublishContainerEvent(env, controllers.EventContainerStopped, cn, nil)
}

func Announcements(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	announcements, err := announcement.FindByHostID(env.DB, h.ID)
	if err != nil {
		return err
	}

	serialized := serializers.SerializeHostAnnouncements(announcements)

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"announcements": serialized,
	}

	return nil
}

func CanCreateContainers(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	can, err := h.CanCreateMoreContainers(env.DB, env.User)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"can_create": can,
	}

	return nil
}

func transitionHostState(d *db.Database, h *host.Host, newState string, initiatorID int64) error {
	return d.WithTransaction(func(tx *sqlx.Tx) error {
		origState := h.State
		if err := h.UpdateState(tx, newState); err != nil {
			if err == host.ErrUpdateFailed {
				return errHostLocked
			}
			return err
		}

		return hosttransition.Record(tx, h, origState, *h.UpdatedAt, initiatorID)
	})
}

func trackWorkspaces(env *middleware.Env, userID int64) {
	plans := []string{}
	err := env.DB.Select(&plans, `
		SELECT plans.name FROM hosts
		JOIN plans on hosts.plan_id = plans.id
		WHERE terminated_at IS NULL
		AND user_id = $1
		ORDER BY hosts.created_at;
	`, userID)
	if err != nil && err != sql.ErrNoRows {
		env.Logger.Errorf("hosts: Could not track user %d workspaces: %s", userID, err)
		return
	}
	env.Tracker.SetProfileProps("set", userID, map[string]interface{}{
		"workspaces_plans": plans,
	}, env.Logger)
}

func getTransientContainerSlugs(cns []*container.Container) string {
	transientContainers := make([]*container.Container, 0)
	for _, cn := range cns {
		if cn.TempState.Valid {
			transientContainers = append(transientContainers, cn)
		}
	}

	if len(transientContainers) > 0 {
		var buffer bytes.Buffer
		for index, cn := range transientContainers {
			buffer.WriteString(cn.Slug)
			if index < len(transientContainers)-1 {
				buffer.WriteString(",")
			}
		}
		return buffer.String()
	}

	return ""
}
