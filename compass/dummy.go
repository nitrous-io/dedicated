package main

import (
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
)

type DummyNavigator struct {
	domain string
	data   map[string]string
}

func (d *DummyNavigator) Name() string {
	return "dummy"
}

func (d *DummyNavigator) Lookup(name string) ([]dns.RR, error) {
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

func (d *DummyNavigator) NewSOA() []dns.RR {
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
	}
}

func NewDummyNavigator(domain string, config map[string]string) (*DummyNavigator, error) {
	return &DummyNavigator{domain, config}, nil
}
