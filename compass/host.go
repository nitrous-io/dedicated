package main

import (
	"database/sql"
	"fmt"
	"net"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/miekg/dns"
)

type Host struct {
	Id        int
	Slug      string
	IpAddress string
}

type HostNavigator struct {
	domain string
	db     *sql.DB
}

func (n *HostNavigator) Name() string {
	return "host"
}

func (n *HostNavigator) Lookup(name string) ([]dns.RR, error) {
	if !SameRootDomain(name, n.domain) {
		return nil, fmt.Errorf("can't handle domain %v, mine is: '%s'", name, n.domain)
	}
	labels := dns.SplitDomainName(name)
	llabel := len(labels)
	if llabel < 3 {
		return nil, fmt.Errorf("missing host slug %v", name)
	}

	// strings.ToLower is safe here because we expect name to have been
	// converted to Punycode when necessary.
	slug := strings.ToLower(labels[llabel-3])
	h, err := n.find(slug)
	if err != nil {
		return nil, fmt.Errorf("Invalid slug from domain %v", name)
	}
	return []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    defaultTTL,
			},
			A: net.ParseIP(h.IpAddress),
		},
	}, nil
}

func (n *HostNavigator) find(slug string) (*Host, error) {
	h := new(Host)
	err := n.db.QueryRow(
		"SELECT id, slug, ip_address FROM hosts WHERE slug = $1 LIMIT 1",
		slug).Scan(&h.Id, &h.Slug, &h.IpAddress)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (n *HostNavigator) NewSOA() []dns.RR {
	return []dns.RR{
		&dns.SOA{
			Hdr:     dns.RR_Header{Name: n.domain, Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: defaultTTL},
			Ns:      "ns1." + n.domain,
			Mbox:    "admin." + n.domain,
			Serial:  uint32(time.Now().Truncate(time.Hour).Unix()),
			Refresh: 28800,
			Retry:   7200,
			Expire:  604800,
			Minttl:  defaultTTL,
		},
	}
}

func NewHostNavigator(domain string, config map[string]string) (*HostNavigator, error) {
	db, err := sql.Open("postgres", config["database_url"])
	if err != nil {
		return nil, fmt.Errorf("Boxhost: failed to get db connection %v", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Boxhost: failed to get db connection %v", err)
	}
	return &HostNavigator{
		domain: domain,
		db:     db,
	}, nil
}
