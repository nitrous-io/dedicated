package main

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

func ensureEndsWithDot(domain string) string {
	if strings.HasSuffix(domain, ".") {
		return domain
	}
	return domain + "."
}
