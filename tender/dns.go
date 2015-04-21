package tender

import (
	"errors"
	"net"

	"github.com/miekg/dns"
)

var (
	dnsServers = []string{"8.8.8.8", "4.4.4.4"}
	dnsPort    = "53"
)

func InitDNS() {
	config, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err == nil {
		if len(config.Servers) > 0 {
			dnsServers = config.Servers
		}
		if config.Port != "" {
			dnsPort = config.Port
		}
	}
}

func resolveDNSCname(host string) (*dns.CNAME, error) {
	host = dns.Fqdn(host)
	m := dns.Msg{}
	m.SetQuestion(host, dns.TypeCNAME)
	m.RecursionDesired = true
	c := dns.Client{}
	r, _, err := c.Exchange(&m, net.JoinHostPort(dnsServers[0], dnsPort))
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, errors.New("nil response from c.Exchange()")
	}
	if r.Rcode != dns.RcodeSuccess {
		return nil, errors.New("invalid r.Rcode")
	}
	for _, a := range r.Answer {
		if cname, ok := a.(*dns.CNAME); ok {
			return cname, nil
		}
	}
	return nil, errors.New("didn't find CNAME response")
}
