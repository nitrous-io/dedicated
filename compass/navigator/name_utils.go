package navigator

import (
	"strings"

	"github.com/miekg/dns"
)

func SameRootDomain(request string, expected string) bool {
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

// RootDomain returns the root domain (i.e. the 2 right-most labels) name string.
// E.g. "foobar.nitrousproxy.com." returns "nitrousproxy.com."
func RootDomain(d string) string {
	labels := dns.SplitDomainName(d)
	if len(labels) < 2 {
		return ""
	}
	return strings.Join(labels[len(labels)-2:], ".") + "."
}
