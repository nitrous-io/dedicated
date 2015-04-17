package hosts

import (
	"database/sql"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/freetier"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/keypairs"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
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
	errMultipleRequests         = httperror.New(http.StatusConflict, "conflict", "another workspace creation request is currently being processed")
	errHostSlugTaken            = httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken")
	errRegionDisabled           = httperror.New(http.StatusBadRequest, "region_disabled", "region is disabled")
	errBasicAuthFailed          = httperror.New(http.StatusUnauthorized, "unauthorized", "")
)

var HostbuilderClient *hostbuilderclient.Hostbuilder
var defaultProvider string
var aesKey string
var fv, dv string

func init() {
	hostbuilderURL := envvars.HostBuilderURL
	if hostbuilderURL == "" {
		panic("HOSTBUILDER_URL is not set")
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)

	aesKey = envvars.AESKey
	if aesKey == "" {
		panic("AES_KEY is not defined")
	}

	defaultProvider = envvars.DefaultProvider
	if defaultProvider == "" {
		panic("DEFAULT_PROVIDER is not set")
	}

	fv := envvars.FreighterdVersion
	if fv == "" {
		panic("FREIGHTERD_VERSION is not set")
	}

	dv := envvars.DockerVersion
	if dv == "" {
		panic("DOCKER_VERSION is not set")
	}
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hosts, err := host.FindAllByUser(env.DB, env.User)
	if err != nil {
		return err
	}

	hostsResp := make([]*serializers.Host, len(hosts))
	for i, h := range hosts {
		hostsResp[i] = serializers.SerializeHost(h)
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
				h.UpgradeAvailable = host.UpgradeAvailableHard
			} else if h.FreighterdVersion != "" && ver.FreighterdVersion != h.FreighterdVersion {
				h.UpgradeAvailable = host.UpgradeAvailableSoft
			}
			if env.User.Admin {
				h.LiveDockerVersion = ver.DockerVersion
				h.LiveFreighterdVersion = ver.FreighterdVersion
			}
		}
	}

	if !envvars.EnableSelfUpgrade && !env.User.Admin {
		h.UpgradeAvailable = ""
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

	publishHostEvent(env, controllers.EventHostUpdated, h)
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
	// If provider identifier is empty, don't attempt to delete the host in hostbuilder
	// Provider identifier can be empty for hosts that are dangling in born or orphan states.
	if h.ProviderIdentifier.String != "" {
		_, err := HostbuilderClient.Delete(h.Provider, h.MachineConfig(), hostbuilderHeaders(env))
		if err != nil {
			return err
		}
	}

	origState := h.State
	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}
	if err := h.Delete(tx); err != nil {
		tx.Rollback()
		return err
	}
	err = hosttransition.Record(tx, h, origState, *h.TerminatedAt)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err := container.DeleteAllByHostID(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err = keypairs.Delete(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := cert.Delete(tx, h.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostDestroyed, h)
	serializeHost(env, h)
	return nil
}

func Start(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	if h.State != host.StateStopped {
		return errHostStartInvalidState
	}

	db := env.DB

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
	h.State = host.StateStarting

	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}
	if err := h.Update(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := hosttransition.Record(tx, h, origState, *h.UpdatedAt); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	publishHostEvent(env, controllers.EventHostStarting, h)

	info, err := HostbuilderClient.Start(h.Provider, h.MachineConfig(), hostbuilderHeaders(env))
	if err != nil {
		// Rollback to previous state, log any errors while rolling back, but
		// return original hostbuilderclient error.
		h.State = origState
		tx, err := db.Beginx()
		if err != nil {
			return err
		}
		if err := h.Update(tx); err != nil {
			tx.Rollback()
			env.Logger.Errorf("hosts.Start: Unable to reset host state: %v", err)
		}
		if err := hosttransition.Record(tx, h, host.StateStarting, *h.UpdatedAt); err != nil {
			tx.Rollback()
			env.Logger.Errorf("hosts.Start: Unable to record transition: %v", err)
		}
		if err := tx.Commit(); err != nil {
			env.Logger.Errorf("hosts.Start: Failed to commit transaction: %v", err)
		}

		publishHostEvent(env, controllers.EventHostUpdated, h)
		return err
	}

	h.State = host.StateRunning
	h.IPAddress = sql.NullString{info.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}

	tx, err = db.Beginx()
	if err != nil {
		return err
	}
	if err := h.Update(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := hosttransition.Record(tx, h, host.StateStarting, *h.UpdatedAt); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostStarted, h)
	serializeHost(env, h)
	return nil
}

func Stop(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	force, _ := strconv.ParseBool(r.FormValue("force"))
	if !force && h.State != host.StateRunning {
		return errHostStopInvalidState
	}

	origState := h.State
	h.State = host.StateStopping

	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}
	if err := h.Update(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := hosttransition.Record(tx, h, origState, *h.UpdatedAt); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	publishHostEvent(env, controllers.EventHostStopping, h)

	if _, err = HostbuilderClient.Stop(h.Provider, h.MachineConfig(), hostbuilderHeaders(env)); err != nil {
		// Rollback to previous state, log any errors while rolling back, but
		// return original hostbuilderclient error.
		h.State = origState
		tx, err := env.DB.Beginx()
		if err != nil {
			return err
		}
		if err := h.Update(tx); err != nil {
			tx.Rollback()
			env.Logger.Errorf("hosts.Stop: Unable to reset host state: %v", err)
		}
		if err := hosttransition.Record(tx, h, host.StateStopping, *h.UpdatedAt); err != nil {
			tx.Rollback()
			env.Logger.Errorf("hosts.Stop: Unable to record transition: %v", err)
		}
		if err := tx.Commit(); err != nil {
			env.Logger.Errorf("hosts.Stop: Failed to commit transaction: %v", err)
		}

		publishHostEvent(env, controllers.EventHostUpdated, h)
		return err
	}

	h.State = host.StateStopped

	tx, err = env.DB.Beginx()
	if err != nil {
		return err
	}
	if err := h.Update(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := hosttransition.Record(tx, h, host.StateStopping, *h.UpdatedAt); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	publishContainersStopped(env, h)
	publishHostEvent(env, controllers.EventHostStopped, h)
	serializeHost(env, h)
	return nil
}

func Restart(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	force, _ := strconv.ParseBool(r.FormValue("force"))
	if !force && h.State != host.StateRunning {
		return errHostRestartInvalidState
	}

	origState := h.State
	h.State = host.StateRestarting

	tx, err := env.DB.Beginx()
	if err != nil {
		return err
	}
	if err := h.Update(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := hosttransition.Record(tx, h, origState, *h.UpdatedAt); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	publishHostEvent(env, controllers.EventHostRestarting, h)

	info, err := HostbuilderClient.Restart(h.Provider, h.MachineConfig(), hostbuilderHeaders(env))
	if err != nil {
		// Rollback to previous state, log any errors while rolling back, but
		// return original hostbuilderclient error.
		h.State = origState
		tx, err := env.DB.Beginx()
		if err != nil {
			return err
		}
		if err := h.Update(tx); err != nil {
			tx.Rollback()
			env.Logger.Errorf("hosts.Restart: Unable to reset host state: %v", err)
		}
		if err := hosttransition.Record(tx, h, host.StateRestarting, *h.UpdatedAt); err != nil {
			tx.Rollback()
			env.Logger.Errorf("hosts.Restart: Unable to record transition: %v", err)
		}
		if err := tx.Commit(); err != nil {
			env.Logger.Errorf("hosts.Stop: Failed to commit transaction: %v", err)
		}

		publishHostEvent(env, controllers.EventHostUpdated, h)
		return err
	}

	h.State = host.StateRunning
	h.IPAddress = sql.NullString{info.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}

	tx, err = env.DB.Beginx()
	if err != nil {
		return err
	}
	if err := h.Update(tx); err != nil {
		tx.Rollback()
		return err
	}

	err = hosttransition.Record(tx, h, host.StateRestarting, *h.UpdatedAt)
	if err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	publishContainersStopped(env, h)
	publishHostEvent(env, controllers.EventHostRestarted, h)
	serializeHost(env, h)
	return nil
}

func Upgrade(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	cl, err := controllers.NewFreighterClient(env.DB, h, env.UUID)
	if err != nil {
		return err
	}

	if err := cl.UpgradeServer(&freighterclient.HostVersions{
		DockerVersion:     h.DockerVersion,
		FreighterdVersion: h.FreighterdVersion,
	}); err != nil {
		if err == freighterclient.ErrAlreadyUpgraded {
			publishHostEvent(env, controllers.EventHostUpgraded, h)
			serializeHost(env, h)
			return nil
		}
		env.Logger.Errorf("failed to upgrade server, err: %v", err)
		return httperror.New(http.StatusInternalServerError, "internal_server_error", "an error occurred while attempting to upgrade your workspace")
	}

	h.State = host.StateUpgrading
	if err := h.Update(env.DB); err != nil {
		return err
	}

	publishHostEvent(env, controllers.EventHostUpgrading, h)
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

func hostbuilderHeaders(env *middleware.Env) map[string]string {
	return map[string]string{
		constants.NitrousUUIDHeader: env.UUID,
	}
}

func serializeHost(env *middleware.Env, h *host.Host) {
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}
}

func publishHostEvent(env *middleware.Env, name controllers.EventName, h *host.Host) {
	env.Events.Publish(
		controllers.NewEvent(name, map[string]*serializers.Host{
			"host": serializers.SerializeHost(h),
		}),
		env.User.Topic(),
	)
}

func publishContainersStopped(env *middleware.Env, h *host.Host) {
	cns, err := container.FindAllByHostID(env.DB, h.ID)
	if err != nil {
		env.Logger.Errorf("hosts.Stop: Unable to list host containers: %v", err)
		return
	}
	for _, cn := range cns {
		cn.State = container.StateStopped
		containers.PublishContainerEvent(env, controllers.EventContainerStopped, cn, nil)
	}
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
