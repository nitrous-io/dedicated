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

	eligible := func(he *groupedhosts.HostEntry) bool {
		if f := r.FormValue("region"); f != "" {
			if he.DBHost.RegionIdentifier.String != f {
				return false
			}
		}
		if f := r.FormValue("state"); f != "" {
			if he.DBHost.State != f {
				return false
			}
		}

		// the default filter that is always applied is "recently used"
		// to turn it off, include query param 'recent=false'
		if f := r.FormValue("recent"); f != "false" {
			if !he.DBHost.LastPingedAt.Valid {
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
