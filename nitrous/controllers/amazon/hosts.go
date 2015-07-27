package amazon

import (
	"net/http"
	"time"

	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/groupedhosts"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

const (
	recentlyUsedPeriod = 24 * 7 * time.Hour
)

// routes
var (
	Hosts       = admin.VerifyAdmin(hosts)
	Healthcheck = admin.VerifyAdminOrService(healthcheck)
)

var HostbuilderClient = provisioner.HostbuilderClient

func hosts(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hostList, err := groupedhosts.ListHosts(env.DB, HostbuilderClient, "amazon")
	if err != nil {
		return err
	}

	// we filter healthy hosts so the admin hosts frontend is faster
	var filtered []*groupedhosts.HostEntry
	regionFilter, stateFilter, recentFilter := r.FormValue("region"), r.FormValue("state"), r.FormValue("recent")

	eligible := func(he *groupedhosts.HostEntry) bool {
		if regionFilter != "" {
			if he.DBHost.RegionIdentifier.String != regionFilter {
				return false
			}
		}
		if stateFilter != "" {
			if he.DBHost.State != stateFilter {
				return false
			}
		}

		// the default filter that is always applied is "recently used"
		// to turn it off, include query param 'recent=false'
		if recentFilter != "false" {
			// LastPingedAt is not populated in two circumstances:
			// 1. very old hosts which have not had a freighter upgrade or been started
			// 2. newly created hosts
			// We want to include #2 while excluding #1, hence the check against CreatedAt
			if !he.DBHost.LastPingedAt.Valid && time.Since(*he.DBHost.CreatedAt) > recentlyUsedPeriod {
				return false
			}
			return time.Since(he.DBHost.LastPingedAt.Time) < recentlyUsedPeriod
		}
		return true
	}

	for _, he := range hostList.HealthyHosts {
		if eligible(he) {
			filtered = append(filtered, he)
		}
	}

	hostList.HealthyHosts = filtered

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hosts": serializers.SerializeHostList(hostList),
	}

	return nil
}

func healthcheck(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hostList, err := groupedhosts.ListHosts(env.DB, HostbuilderClient, "amazon")
	if err != nil {
		return httperror.New(http.StatusInternalServerError, "health_check_error", err.Error())
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"healthcheck": serializers.SerializeHealthcheck(hostList),
	}

	return nil
}
