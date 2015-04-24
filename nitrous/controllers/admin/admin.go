package admin

import (
	"database/sql"
	"net/http"
	"net/url"
	"regexp"

	"github.com/gorilla/mux"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/constants"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	// matches the agreed format for testing images `create-test-#{region}-#{image_id}-#{unix_epoch}`
	nonNitrousHosts = regexp.MustCompile(`(?i)^create-test-(\w+)-(\d+)-(\d{10,})$`)
)

var (
	errUserNotFound               = httperror.New(http.StatusBadRequest, "user_not_found", "cannot find a user for the given email or username")
	errProviderRequired           = httperror.New(http.StatusBadRequest, "invalid_request", "provider is required")
	errProviderIdentifierRequired = httperror.New(http.StatusBadRequest, "invalid_request", "provider_identifier is required")
	errHostIsNotUnaccounted       = httperror.New(http.StatusBadRequest, "invalid_request", "host is known in DB, cannot terminate directly")
	errFreighterdVersionRequired  = httperror.New(http.StatusBadRequest, "invalid_request", "freighterd version required")
)

var (
	SwitchUser               = VerifyAdmin(switchUser)
	Hosts                    = VerifyAdmin(hosts)
	TerminateUnaccountedHost = VerifyAdmin(terminateUnaccountedHost)
	UpdateHost               = VerifyAdmin(updateHost)
)

var HostbuilderClient = provisioner.HostbuilderClient

func switchUser(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	login := r.FormValue("login") // this can be either username or email
	u, err := user.FindByUsernameOrEmail(db, login)
	if err != nil {
		if err == sql.ErrNoRows {
			return errUserNotFound
		}
		return err
	}

	return env.Login(u, w)
}

type liveHost struct {
	Provider       string `json:"provider"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort uint16 `json:"freighterd_port"`
	State          string `json:"state"`
}

func hosts(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	provider := r.FormValue("provider")
	if provider == "" {
		return errProviderRequired
	}

	dbHosts, err := host.FindAllByProvider(env.DB, provider)
	if err != nil {
		return err
	}

	hostbuilderHosts, err := HostbuilderClient.List(provider)
	if err != nil {
		return err
	}

	liveHosts := []*liveHost{}
	for _, h := range hostbuilderHosts {
		if nonNitrousHosts.MatchString(h.Name) {
			continue
		}
		lh := &liveHost{
			Provider:       h.Provider,
			ID:             h.ID,
			Name:           h.Name,
			IPAddress:      h.IPAddress,
			FreighterdPort: h.FreighterdPort,
			State:          h.State,
		}
		liveHosts = append(liveHosts, lh)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"db_hosts":   serializers.SerializeAdminHosts(dbHosts),
		"live_hosts": liveHosts,
	}

	return nil
}

func terminateUnaccountedHost(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	provider := r.FormValue("provider")
	if provider == "" {
		return errProviderRequired
	}
	provID := r.FormValue("provider_identifier")
	if provID == "" {
		return errProviderIdentifierRequired
	}

	config := hostbuilderclient.Config{"id": provID}

	if region := r.FormValue("region"); region != "" {
		config["region_id"] = region
	}

	if ip := r.FormValue("ip_address"); ip != "" {
		config["ip_address"] = ip
	}

	var count int
	err := env.DB.QueryRow(`
		SELECT COUNT(*) FROM hosts
		INNER JOIN provider_templates pt
			ON hosts.provider_template_id = pt.id
		WHERE
			hosts.provider_identifier = $1 AND hosts.terminated_at IS NULL
			AND pt.provider = $2`,
		provID, provider).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return errHostIsNotUnaccounted
	}

	headers := http.Header{}
	headers.Set(constants.NitrousUUIDHeader, env.UUID)
	urlArgs := url.Values{}
	urlArgs.Set("uid", "admin")
	resp, err := HostbuilderClient.Delete(provider, config, headers, urlArgs)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": resp.Status,
	}
	return nil
}

func updateHost(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	freighterdVer := r.FormValue("freighterd_version")
	if freighterdVer == "" {
		return errFreighterdVersionRequired
	}

	h, err := host.FindByIdentifier(db, mux.Vars(r)["slug"])
	if err != nil {
		if err == sql.ErrNoRows {
			return middleware.ErrHostNotFound
		}
		return err
	}

	h.FreighterdVersion = freighterdVer
	if err = h.Update(db); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"host": serializers.SerializeHost(h),
	}
	return nil
}

func VerifyAdmin(fn middleware.Middleware) middleware.Middleware {
	return func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		if !env.User.Admin {
			return httperror.New(http.StatusNotFound, "not_found", "not found")
		}

		return fn(env, w, r)
	}
}
