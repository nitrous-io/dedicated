package navigator

import "github.com/miekg/dns"

func SameRootDomain(request string, expected string) bool {
	r := dns.SplitDomainName(request)
	lr := len(r)
	e := dns.SplitDomainName(expected)
	le := len(e)
	return r[lr-1] == e[le-1] && r[lr-2] == e[le-2]
}
