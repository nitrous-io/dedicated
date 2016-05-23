package customdomains

import (
	"net/http"
	"regexp"
	"strconv"

	"github.com/gorilla/mux"
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
	Delete middleware.Middleware = containers.WithContainerAndHost(delete, containers.TouchHost)

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
	if !isDomainName(domain) {
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

func delete(cn *container.Container, h *host.Host, env *middleware.Env, w http.ResponseWriter, r *http.Request) error {
	customDomainID, err := strconv.ParseInt(mux.Vars(r)["cdid"], 10, 64)
	if err != nil {
		return errInvalidPort
	}

	if err := customdomain.Destroy(env.DB, cn.ID, customDomainID); err != nil {
		return err
	}

	env.Status = http.StatusOK
	env.Data = map[string]interface{}{
		"status": "ok",
	}
	return nil
}

// Copy from https://github.com/golang/go/blob/0104a31b8fbcbe52728a08867b26415d282c35d2/src/net/dnsclient.go#L116
func isDomainName(s string) bool {
	// See RFC 1035, RFC 3696.
	if len(s) == 0 {
		return false
	}
	if len(s) > 255 {
		return false
	}

	last := byte('.')
	ok := false // Ok once we've seen a letter.
	partlen := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		default:
			return false
		case 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || c == '_':
			ok = true
			partlen++
		case '0' <= c && c <= '9':
			// fine
			partlen++
		case c == '-':
			// Byte before dash cannot be dot.
			if last == '.' {
				return false
			}
			partlen++
		case c == '.':
			// Byte before dot cannot be dot, dash.
			if last == '.' || last == '-' {
				return false
			}
			if partlen > 63 || partlen == 0 {
				return false
			}
			partlen = 0
		}
		last = c
	}
	if last == '-' || partlen > 63 {
		return false
	}

	return ok
}
