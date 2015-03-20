package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/miekg/dns"
)

type ProxyHost struct {
	Id         int
	Slug       string
	RegionID   int
	RegionSlug string
	ProviderID int
	Provider   string
}

type ProxyNavigator struct {
	domain string
	db     *sql.DB
	config map[string]string
}

func (n *ProxyNavigator) Name() string {
	return "proxy"
}

func (n *ProxyNavigator) Lookup(name string) ([]dns.RR, error) {
	if !SameRootDomain(name, n.domain) {
		return nil, fmt.Errorf("Cant handle domain %v", name)
	}
	labels := dns.SplitDomainName(name)
	llabel := len(labels)
	if llabel < 3 {
		return nil, fmt.Errorf("Missing host slug %v", name)
	}

	// strings.ToLower is safe here because we expect name to have been
	// converted to Punycode when necessary.
	slug := strings.ToLower(labels[llabel-3])
	h, err := n.find(slug)
	if err != nil {
		return nil, fmt.Errorf("Invalid slug from domain %v", name)
	}

	domain := n.config[h.RegionSlug]
	if len(domain) == 0 {
		return nil, fmt.Errorf("Empty proxy for region '%s', please check the config.", h.RegionSlug)
	}
	domain = ensureEndsWithDot(domain)

	cname := fmt.Sprintf("%s.%s", h.Provider, domain)
	return []dns.RR{
		&dns.CNAME{
			Hdr: dns.RR_Header{
				Name:   name,
				Rrtype: dns.TypeCNAME,
				Class:  dns.ClassINET,
				Ttl:    defaultTTL,
			},
			Target: cname,
		},
	}, nil
}

func (n *ProxyNavigator) find(slug string) (*ProxyHost, error) {
	var h ProxyHost
	err := n.db.QueryRow(
		`SELECT hosts.id, hosts.slug, regions.id, regions.slug, provider_templates.id, provider_templates.provider
		FROM hosts, regions, provider_templates WHERE hosts.slug = $1 AND hosts.region_id = regions.id
		AND hosts.provider_template_id = provider_templates.id LIMIT 1`, slug,
	).Scan(&h.Id, &h.Slug, &h.RegionID, &h.RegionSlug, &h.ProviderID, &h.Provider)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (n *ProxyNavigator) NewSOA() []dns.RR {
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

func NewProxyNavigator(domain string, config map[string]string) (*ProxyNavigator, error) {
	domainDot := ensureEndsWithDot(domain)
	db, err := sql.Open("postgres", config["database_url"])
	if err != nil {
		return nil, fmt.Errorf("Boxhost: failed to get db connection %v", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Boxhost: failed to get db connection %v", err)
	}
	return &ProxyNavigator{
		domain: domainDot,
		db:     db,
		config: config,
	}, nil
}
