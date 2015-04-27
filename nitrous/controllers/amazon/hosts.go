package amazon

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/groupedhosts"
	"github.com/nitrous-io/dedicated/nitrous/provisioner"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

// routes
var (
	Hosts       = admin.VerifyAdmin(hosts)
	Healthcheck = admin.VerifyAdmin(healthcheck)
)

var HostbuilderClient = provisioner.HostbuilderClient

func hosts(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hostList, err := groupedhosts.ListHosts(env.DB, HostbuilderClient, "amazon")
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"hosts": serializers.SerializeHostList(hostList),
	}

	return nil
}

func healthcheck(env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	hostList, err := groupedhosts.ListHosts(env.DB, HostbuilderClient, "amazon")
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"healthcheck": serializers.SerializeHealthcheck(hostList),
	}

	return nil
}
