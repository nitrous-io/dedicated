package dummy

import (
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
	n "github.com/nitrous-io/dedicated/compass/navigator"
)

const defaultTTL = uint32(300)

type Dummy struct {
	domain string
	data   map[string]string
}

func (d *Dummy) Name() string {
	return "dummy"
}

func (d *Dummy) Lookup(name string) ([]dns.RR, error) {
	ip, ok := d.data[name]
	if !ok {
		return nil, fmt.Errorf("InvalidRecord: %v", name)
	}
	records := []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    defaultTTL,
			},
			A: net.ParseIP(ip),
		},
	}
	return records, nil
}

func (d *Dummy) SOA(domain string) ([]dns.RR, error) {
	if domain != "" && d.domain != domain {
		return nil, n.ErrDomainMismatch
	}
	return []dns.RR{
		&dns.SOA{
			Hdr:     dns.RR_Header{Name: d.domain, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: defaultTTL},
			Ns:      "ns1." + d.domain,
			Mbox:    "admin." + d.domain,
			Serial:  uint32(time.Now().Truncate(time.Hour).Unix()),
			Refresh: 28800,
			Retry:   7200,
			Expire:  604800,
			Minttl:  defaultTTL,
		},
	}, nil
}

func NewNavigator(domain string, config map[string]string) (n.Navigator, error) {
	return &Dummy{domain, config}, nil
}

func init() {
	n.Navigators["dummy"] = NewNavigator
}
