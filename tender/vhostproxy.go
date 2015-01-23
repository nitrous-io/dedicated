package tender

import (
	"net/http"

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
	}
}

func sameRootDomain(request string, expected string) bool {
	r := dns.SplitDomainName(request)
	lr := len(r)
	e := dns.SplitDomainName(expected)
	le := len(e)
	return r[lr-1] == e[le-1] && r[lr-2] == e[le-2]
}
