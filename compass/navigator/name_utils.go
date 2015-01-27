package navigator

import (
	"strings"

	"github.com/miekg/dns"
)

func SameRootDomain(request string, expected string) bool {
	r := dns.SplitDomainName(request)
	lr := len(r)
	e := dns.SplitDomainName(expected)
	le := len(e)
	return strings.EqualFold(r[lr-1], e[le-1]) && strings.EqualFold(r[lr-2], e[le-2])
}
