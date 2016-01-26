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

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/hostcontrol"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/announcement"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/hosttransition"
	"github.com/nitrous-io/dedicated/nitrous/models/providertemplate"
	"github.com/nitrous-io/dedicated/nitrous/models/respec"
	"github.com/nitrous-io/dedicated/nitrous/models/subscription"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/dedicated/nitrous/subscriptions"
)

var (
	errSizeRequired                = httperror.New(http.StatusBadRequest, "invalid_request", "size is required")
	errNameRequired                = httperror.New(http.StatusBadRequest, "invalid_request", "name is required")
	errRegionRequired              = httperror.New(http.StatusBadRequest, "invalid_request", "region is required")
	errPlanNotFound                = httperror.New(http.StatusBadRequest, "invalid_request", "no matching plan found")
	errRegionNotFound              = httperror.New(http.StatusBadRequest, "invalid_request", "no matching region found for the given name")
	errProvidertemplateNotFound    = httperror.New(http.StatusBadRequest, "invalid_request", "no matching templates found for the given plan and region combination")
	errHostStartInvalidState       = httperror.New(http.StatusBadRequest, "invalid_state", "workstation is not stopped")
	errHostStopInvalidState        = httperror.New(http.StatusBadRequest, "invalid_state", "workstation is not running")
	errHostRespecInvalidState      = httperror.New(http.StatusBadRequest, "invalid_state", "workstation must be running or stopped to upgrade")
	errHostUpdateInvalidState      = httperror.New(http.StatusBadRequest, "invalid_state", "workstation is not running")
	errHostRestartInvalidState     = httperror.New(http.StatusBadRequest, "invalid_state", "workstation is not running")
	errHostNotRunning              = httperror.New(http.StatusBadRequest, "invalid_state", "workstation is not running")
	errHostCreationNotAllowed      = httperror.New(http.StatusForbidden, "request_not_allowed", "not allowed to create more workstations")
	errMultipleRequests            = httperror.New(http.StatusConflict, "conflict", "another workstation creation request is currently being processed")
	errHostSlugTaken               = httperror.New(http.StatusBadRequest, "validation_failed", "slug is taken")
	errHostNoPendingRespec         = httperror.New(http.StatusBadRequest, "invalid_request", "workstation has no upgrades available")
	errRegionDisabled              = httperror.New(http.StatusBadRequest, "region_disabled", "region is disabled")
	errBasicAuthFailed             = httperror.New(http.StatusUnauthorized, "unauthorized", "")
	errSubscriptionStillActive     = httperror.New(http.StatusBadRequest, "invalid_request", "cannot terminate a workstation with an active subscription")
	errFreeUsageExceededForStart   = httperror.New(http.StatusForbidden, "forbidden", "you cannot start this workstation because you've exceeded your quota")
	errFreeUsageExceededForRestart = httperror.New(http.StatusForbidden, "forbidden", "you cannot restart this workstation because you've exceeded your quota")
)

var (
	HostbuilderClient = provisioner.HostbuilderClient
	defaultProvider   string
	aesKey            string
	subProvider       subscriptions.Provider
)

func init() {
	aesKey = envvars.AESKey
	if aesKey == "" {
		panic("AES_KEY is not defined")
	}

	defaultProvider = envvars.DefaultProvider
	if defaultProvider == "" {
		panic("DEFAULT_PROVIDER is not set")
	}

	subProvider = subscriptions.NewProvider()
}

func Index(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hosts, err := host.FindAllByUser(env.DB, env.User.ID)
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
	// middleware uses host.FindByIdentifier or host.FindShared which do not
	// check for deleted subscription
	if h.SubscriptionID.Valid {
		s, err := subscription.Find(env.DB, h.SubscriptionID.Int64)
		if err != nil {
			return err
		}
		if s.State == subscription.StateDeleted {
			return httperror.New(http.StatusNotFound, "not_found", "not found")
		}
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
		if _, ok := err.(*freighterclient.RequestFailedError); ok {
			return httperror.New(http.StatusGatewayTimeout, "connectivity_error", "cannot connect to workspace")
		}
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"stats": serializers.SerializeHostInfo(info),
	}
	return nil
}

func Destroy(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	// We only allow host to be terminated if:
	// 1. it is a free host (in which case we cancel the subscription), or
	// 2. it is a paid host whose subscription has already been canceled.
	// 3. the subscription supports multiple hosts
	if h.SubscriptionID.Valid {
		sub, err := subscription.Find(env.DB, h.SubscriptionID.Int64)
		if err != nil {
			return err
		}

		if !sub.MultiHost && !(sub.CancelPending || sub.State == subscription.StateDeleted) {
			if h.Free {
				if err := subProvider.Cancel(env.DB, sub); err != nil {
					return err
				}
			} else {
				return errSubscriptionStillActive
			}
		}
	}

	origState := h.State
	if err := h.UpdateState(env.DB, host.StateTerminating); err != nil {
		return err
	}
	PublishHostEvent(env, controllers.EventHostDestroying, h)

	err := env.DB.WithTransaction(func(tx *sqlx.Tx) error {
		return hostcontrol.Terminate(tx, h, hostbuilderHeaders(env), env.User.ID, origState)
	})

	if err != nil {
		return err
	}

	if env.User.Admin && r.FormValue("reassign") == "true" {
		_, err = env.DB.Exec("UPDATE hosts SET user_id = $1 WHERE id = $2;", env.User.ID, h.ID)
		if err != nil {
			logrus.Errorf("Error reassigning host, err: %+v", err)
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
	u := env.User

	// If this is a free host, ensure user has remaining usage quota.
	if h.Free && !u.HasUsageRemaining() {
		return errFreeUsageExceededForStart
	}

	origState := h.State
	if err := hostcontrol.UpdateAndRecordState(db, h, host.StateStarting, u.ID); err != nil {
		return err
	}

	PublishHostEvent(env, controllers.EventHostStarting, h)

	urlArgs := host.URLArgsFromHost(h)
	info, err := HostbuilderClient.Start(h.Provider, h.MachineConfig(), hostbuilderHeaders(env), urlArgs)
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			logrus.Errorf("hostbuilder timeout starting host: %v", e)
		}

		// Rollback to previous state, log any errors while rolling back, but
		// return original hostbuilderclient error.
		if err := hostcontrol.UpdateAndRecordState(db, h, origState, u.ID); err != nil {
			logrus.Errorf("hosts.Start: Unable to reset host state: %v", err)
		}

		PublishHostEvent(env, controllers.EventHostUpdated, h)
		return err
	}

	h.State = host.StateRunning
	h.ProviderIdentifier = sql.NullString{info.ID, true}
	h.IPAddress = sql.NullString{info.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}
	if err := db.WithTransaction(func(tx *sqlx.Tx) error {
		columnsToUpdate := []string{"state", "provider_identifier", "ip_address", "freighterd_port"}
		if err := h.UpdateSpecifiedColumns(tx, columnsToUpdate); err != nil {
			return err
		}
		return hosttransition.Record(tx, h, host.StateStarting, *h.UpdatedAt, u.ID)
	}); err != nil {
		return err
	}

	PublishHostEvent(env, controllers.EventHostStarted, h)

	serializeHost(env, h)
	return nil
}

func Stop(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
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

	if err := hostcontrol.StopWithQueue(env.DB, h, hostbuilderHeaders(env), evHandler, env.User.ID); err != nil {
		return err
	}

	env.SetEmptyResponseBody()

	return nil
}

func Restart(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB
	u := env.User

	force, _ := strconv.ParseBool(r.FormValue("force"))
	if !force && h.State != host.StateRunning {
		return errHostRestartInvalidState
	}

	// If this is a free host, ensure user has remaining usage quota.
	if h.Free && !u.HasUsageRemaining() {
		return errFreeUsageExceededForRestart
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
	if err := hostcontrol.UpdateAndRecordState(db, h, host.StateRestarting, u.ID); err != nil {
		return err
	}

	PublishHostEvent(env, controllers.EventHostRestarting, h)

	urlArgs := host.URLArgsFromHost(h)
	info, err := HostbuilderClient.Restart(h.Provider, h.MachineConfig(), hostbuilderHeaders(env), urlArgs)
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			logrus.Errorf("hostbuilder timeout restarting host: %v", e)
		}

		// Rollback to previous state, log any errors while rolling back, but
		// return original hostbuilderclient error.
		if err := hostcontrol.UpdateAndRecordState(db, h, origState, u.ID); err != nil {
			logrus.Errorf("hosts.Restart: Unable to restore host state: %v", err)
		}

		PublishHostEvent(env, controllers.EventHostUpdated, h)
		return err
	}

	h.State = host.StateRunning
	h.IPAddress = sql.NullString{info.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}

	if err := db.WithTransaction(func(tx *sqlx.Tx) error {
		columnsToUpdate := []string{"state", "ip_address", "freighterd_port"}
		if err := h.UpdateSpecifiedColumns(tx, columnsToUpdate); err != nil {
			return err
		}
		return hosttransition.Record(tx, h, host.StateRestarting, *h.UpdatedAt, u.ID)
	}); err != nil {
		return err
	}

	PublishHostEvent(env, controllers.EventHostRestarted, h)
	for _, cn := range cns {
		state := container.State(cn.TempState.Int64)
		if !cn.TempState.Valid || (cn.TempState.Valid && state != container.StateStopping) {
			PublishContainerStopped(env, cn)
		}
	}

	serializeHost(env, h)
	return nil
}

func SoftRestart(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	PublishHostEvent(env, controllers.EventHostSoftRestarting, h)

	if h.State != host.StateRunning {
		return errHostRestartInvalidState
	}

	urlArgs := host.URLArgsFromHost(h)
	info, err := HostbuilderClient.SoftRestart(h.Provider, h.MachineConfig(), hostbuilderHeaders(env), urlArgs)
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			logrus.Errorf("hostbuilder timeout soft restarting host: %v", e)
		}

		PublishHostEvent(env, controllers.EventHostUpdated, h)
		return err
	}

	h.State = host.StateRunning
	h.IPAddress = sql.NullString{info.IPAddress, true}
	h.FreighterdPort = sql.NullInt64{int64(info.FreighterdPort), true}

	PublishHostEvent(env, controllers.EventHostSoftRestarted, h)
	serializeHost(env, h)
	return nil
}

func Respec(h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	if !(h.State == host.StateRunning || h.State == host.StateStopped) {
		return errHostRespecInvalidState
	}

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

	if err := h.UpdateState(db, host.StateRespeccing); err != nil {
		return err
	}

	if err := rs.UpdateState(db, respec.StateProcessing); err != nil {
		return err
	}

	h.ProviderTemplate = pt // So that MachineConfig() returns config with the new provider template.
	config := h.MachineConfig()

	if _, err := HostbuilderClient.Respec(h.Provider, config, hostbuilderHeaders(env), host.URLArgsFromHost(h)); err != nil {
		return err
	}

	PublishHostEvent(env, controllers.EventHostRespeccing, h)
	env.Status = http.StatusAccepted
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
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
		logrus.Warnf("cannot decode Basic Auth header, err: %v", err)
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
		logrus.Warnf("cannot decode parse host ID from %v, err: %v", pair[0], err)
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
			logrus.Errorf("Error retrieving owner %d for host %d, err: %+v", h.UserID, h.ID, err)
		}
	}
	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}
}

func PublishHostEvent(env *middleware.Env, name controllers.EventName, h *host.Host) {
	u := env.User
	if u.ID != h.UserID {
		var err error
		u, err = user.FindByID(env.DB, h.UserID)
		if err != nil {
			logrus.Errorf("Error retrieving owner %d for host %d, err: %+v", h.UserID, h.ID, err)
		}
	}
	env.Events.Publish(
		controllers.NewEvent(name, map[string]*serializers.Host{
			"host": serializers.SerializeHost(h),
		}),
		u.Topic(),
	)
}

func PublishContainerstopped(env *middleware.Env, h *host.Host) {
	cns, err := container.FindAllByHostID(env.DB, h.ID)
	if err != nil {
		logrus.Errorf("hosts.Stop: Unable to list host containers: %v", err)
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
		logrus.Errorf("hosts: Could not track user %d workspaces: %s", userID, err)
		return
	}
	env.Tracker.SetProfileProps("set", userID, map[string]interface{}{
		"workspaces_plans": plans,
	})
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
