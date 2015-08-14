package tender

import (
	"net/http"
	"strings"

	"github.com/miekg/dns"
)

type VhostServer struct {
	FreighterBaseDomain  string
	FreighterProxyServer http.Handler
	PreviewBaseDomain    string
	PreviewProxyServer   http.Handler
}

func (s *VhostServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if sameRootDomain(r.Host, s.FreighterBaseDomain) {
		s.FreighterProxyServer.ServeHTTP(w, r)
	} else if sameRootDomain(r.Host, s.PreviewBaseDomain) {
		s.PreviewProxyServer.ServeHTTP(w, r)
	} else {
		http.Error(w, "cannot handle this request", http.StatusForbidden)
	}
}

// also in compass/navigator/name_utils.go
func sameRootDomain(request string, expected string) bool {
	r := dns.SplitDomainName(request)
	lr := len(r)
	if lr < 2 {
		return false
	}
	e := dns.SplitDomainName(expected)
	le := len(e)
	if le < 2 {
		return false
	}
	return strings.EqualFold(r[lr-1], e[le-1]) && strings.EqualFold(r[lr-2], e[le-2])
}
