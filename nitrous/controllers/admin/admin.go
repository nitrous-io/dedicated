package admin

import (
	"net/http"

	"github.com/nitrous-io/dedicated/hostbuilder/state"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/models/user"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
	"github.com/nitrous-io/hostbuilder"
)

var (
	errUserNotFound     = httperror.New(http.StatusBadRequest, "user_not_found", "cannot find a user for the given email or username")
	errProviderRequired = httperror.New(http.StatusBadRequest, "invalid_request", "provider is required")
)

var (
	SwitchUser = verifyAdmin(switchUser)
	Hosts      = verifyAdmin(hosts)
)

var HostbuilderClient *hostbuilderclient.Hostbuilder

func init() {
	hostbuilderURL := envvars.HostBuilderURL
	if hostbuilderURL == "" {
		panic("HOSTBUILDER_URL is not set")
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)
}

func switchUser(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	db := env.DB

	login := r.FormValue("login") // this can be either username or email
	u, err := user.FindByUsernameOrEmail(db, login)
	if err != nil {
		return err
	}
	if u == nil {
		return errUserNotFound
	}

	return env.Login(u, w)
}

type unaccountedHost struct {
	Provider       string `json:"provider"`
	ID             string `json:"id"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort uint16 `json:"freighterd_port"`
	State          string `json:"state"`
}

func hosts(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	provider := r.FormValue("provider")
	if provider == "" {
		return errProviderRequired
	}

	// Get hosts from DB and from Hostbuilder and inventory them.
	// Classify into sections:
	//   1. hosts that are in Hostbuilder but not in DB.
	//   2. hosts that are not running (according to DB or Hostbuilder).
	//   3. hosts that are in DB but not in Hostbuilder.
	//   4. hosts that are fine.
	// TODO We might want to mark and group pre-provisioned hosts in each section.
	dbHosts, err := host.FindAllByProvider(env.DB, provider)
	if err != nil {
		return err
	}

	liveHosts, err := HostbuilderClient.List(provider)
	if err != nil {
		return err
	}

	// Group 1: hosts that are in Hostbuilder but not in DB.
	unaccountedHosts := []unaccountedHost{}
	for _, h := range liveHosts {
		if !containsHost(dbHosts, h.ID) {
			uh := unaccountedHost{
				Provider:       h.Provider,
				ID:             h.ID,
				IPAddress:      h.IPAddress,
				FreighterdPort: h.FreighterdPort,
				State:          h.State,
			}
			unaccountedHosts = append(unaccountedHosts, uh)
		}
	}

	// Group 2: hosts that are not running (according to DB or Hostbuilder).
	nonRunningHosts := []host.Host{}
	for _, h := range dbHosts {
		if h.State != host.StateRunning {
			nonRunningHosts = append(nonRunningHosts, h)
			continue
		}

		lh := findHost(liveHosts, h.ProviderIdentifier.String)
		if lh == nil {
			continue
		}

		if lh.State != state.Running.String() {
			nonRunningHosts = append(nonRunningHosts, h)
		}
	}

	// Group 3: hosts that are in DB but not in Hostbuilder.
	danglingHosts := []host.Host{}
	for _, h := range dbHosts {
		if lh := findHost(liveHosts, h.ProviderIdentifier.String); lh == nil {
			danglingHosts = append(danglingHosts, h)
		}
	}

	// Group 4: hosts that are fine.
	okHosts := []host.Host{}
	for _, h := range dbHosts {
		if h.State != host.StateRunning {
			continue
		}

		lh := findHost(liveHosts, h.ProviderIdentifier.String)
		if lh == nil {
			continue
		}

		if lh.State != state.Running.String() {
			continue
		}

		okHosts = append(okHosts, h)
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"unaccounted": unaccountedHosts,
		"non_running": serializers.SerializeHosts(nonRunningHosts),
		"dangling":    serializers.SerializeHosts(danglingHosts),
		"ok":          serializers.SerializeHosts(okHosts),
	}

	return nil
}

func containsHost(hosts []host.Host, id string) bool {
	for _, h := range hosts {
		if h.ProviderIdentifier.String == id {
			return true
		}
	}
	return false
}

func findHost(hosts []*hostbuilder.Response, id string) *hostbuilder.Response {
	for _, h := range hosts {
		if h.ID == id {
			return h
		}
	}
	return nil
}

func verifyAdmin(fn middleware.Middleware) middleware.Middleware {
	return func(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
		if !env.User.Admin {
			return httperror.New(http.StatusNotFound, "not_found", "not found")
		}

		return fn(env, w, r)
	}
}
