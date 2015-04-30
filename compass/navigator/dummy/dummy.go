package dummy

import (
	"fmt"
	"net"

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

func NewNavigator(domain string, config map[string]string) (n.Navigator, error) {
	return &Dummy{domain, config}, nil
}

func init() {
	n.Navigators["dummy"] = NewNavigator
}
