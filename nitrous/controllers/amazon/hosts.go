package amazon

import (
	"net/http"

	"github.com/nitrous-io/dedicated/nitrous/appconfig"
	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/groupedhosts"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var Hosts = admin.VerifyAdmin(hosts)

var HostbuilderClient *hostbuilderclient.Hostbuilder

func init() {
	hostbuilderURL := appconfig.HostBuilderURL
	if hostbuilderURL == "" {
		panic("HOSTBUILDER_URL is not set")
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)
}

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
