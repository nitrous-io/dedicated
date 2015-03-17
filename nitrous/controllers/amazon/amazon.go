package amazon

import (
	"net/http"
	"regexp"

	"github.com/nitrous-io/dedicated/nitrous/controllers/admin"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	// matches the agreed format for testing images `create-test-#{region}-#{image_id}-#{unix_epoch}`
	nonNitrousHosts = regexp.MustCompile(`(?i)^create-test-(\w+)-(\d+)-(\d{10,})$`)
)

var Hosts = admin.VerifyAdmin(hosts)

var HostbuilderClient *hostbuilderclient.Hostbuilder

func init() {
	hostbuilderURL := envvars.HostBuilderURL
	if hostbuilderURL == "" {
		panic("HOSTBUILDER_URL is not set")
	}
	HostbuilderClient = hostbuilderclient.NewHostbuilder(hostbuilderURL)
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
	provider := "amazon"

	dbHosts, err := host.FindAllByProvider(env.DB, provider)
	if err != nil {
		return err
	}

	hostbuilderHosts, err := HostbuilderClient.List(provider)
	if err != nil {
		return err
	}

	liveHosts := []liveHost{}
	for _, h := range hostbuilderHosts {
		if nonNitrousHosts.MatchString(h.Name) {
			continue
		}
		lh := liveHost{
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
