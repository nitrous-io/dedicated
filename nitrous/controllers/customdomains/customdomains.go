package customdomains

import (
	"net/http"
	"regexp"
	"strconv"

	"github.com/lib/pq"
	"github.com/nitrous-io/dedicated/nitrous/controllers/containers"
	"github.com/nitrous-io/dedicated/nitrous/middleware"
	"github.com/nitrous-io/dedicated/nitrous/models/container"
	"github.com/nitrous-io/dedicated/nitrous/models/customdomain"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/pkg/httperror"
	"github.com/nitrous-io/dedicated/nitrous/serializers"
)

var (
	Index  middleware.Middleware = containers.WithContainerAndHost(index, containers.TouchHost)
	Create middleware.Middleware = containers.WithContainerAndHost(create, containers.TouchHost)

	errInvalidDomain        = httperror.New(http.StatusBadRequest, "invalid_request", "Invalid domain")
	errInvalidPort          = httperror.New(http.StatusBadRequest, "invalid_request", "Invalid port -- Please specify port 80, 443 or between 1024 to 9999")
	errInvalidContainerPort = httperror.New(http.StatusBadRequest, "invalid_request", "Invalid container port -- Please specify port between 1024 to 9999")
	errDuplicateDomain      = httperror.New(http.StatusBadRequest, "invalid_request", "Duplicated domain name with same port")
)

func index(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	customDomains, err := customdomain.FindAll(env.DB, cn.ID)
	if err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"custom_domains": serializers.SerializeCustomDomains(customDomains),
	}
	return nil

}

func create(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	domain := r.FormValue("domain")
	protocolRegex := regexp.MustCompile("^http://|^https://")
	domain = protocolRegex.ReplaceAllString(domain, "")
	domainRegExp := regexp.MustCompile(`^(([a-zA-Z]{1})|([a-zA-Z]{1}[a-zA-Z]{1})|([a-zA-Z]{1}[0-9]{1})|([0-9]{1}[a-zA-Z]{1})|([a-zA-Z0-9][a-zA-Z0-9-_]{1,61}[a-zA-Z0-9]))\.([a-zA-Z]{2,6}|[a-zA-Z0-9-]{2,30}\.[a-zA-Z]{2,3})$`)
	if !domainRegExp.MatchString(domain) {
		return errInvalidDomain
	}

	port, err := strconv.ParseInt(r.FormValue("port"), 10, 64)
	if err != nil {
		return errInvalidPort
	}
	if port != 80 && port != 443 && (port < 1024 || port > 9999) {
		return errInvalidPort
	}

	containerPort, err := strconv.ParseInt(r.FormValue("container_port"), 10, 64)
	if err != nil {
		return errInvalidContainerPort
	}
	if containerPort < 1024 || containerPort > 9999 {
		return errInvalidContainerPort
	}

	cd := &customdomain.CustomDomain{
		ContainerID:   cn.ID,
		Domain:        domain,
		Port:          port,
		ContainerPort: containerPort,
	}
	if err := cd.Insert(env.DB); err != nil {
		if e, ok := err.(*pq.Error); ok && e.Code.Name() == "unique_violation" && e.Constraint == "index_custom_domains_on_domain_and_port" {
			return errDuplicateDomain
		}

		return err
	}

	env.Status = http.StatusCreated
	env.Data = map[string]interface{}{
		"status": "created",
	}

	return nil
}
