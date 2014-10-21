package proxy

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/miekg/dns"
	nav "github.com/nitrous-io/compass/navigator"
)

const defaultTTL = uint32(300)

var (
	ErrNotFound = errors.New("RecordNotFound")
)

type Host struct {
	Id         int
	Slug       string
	RegionID   int
	RegionSlug string
}

type Navigator struct {
	domain string
	db     *sql.DB
	config map[string]string
}

func (n *Navigator) Lookup(name string) ([]dns.RR, error) {
	if !nav.SameRootDomain(name, n.domain) {
		return nil, fmt.Errorf("Cant handle domain %v", name)
	}
	labels := dns.SplitDomainName(name)
	llabel := len(labels)
	if llabel < 3 {
		return nil, fmt.Errorf("Missing host slug %v", name)
	}
	slug := labels[llabel-3]
	h, err := n.find(slug)
	if err != nil {
		return nil, fmt.Errorf("Invalid slug from domain %v", name)
	}

	domain := n.config[h.RegionSlug]
	if len(domain) == 0 {
		return nil, fmt.Errorf("Empty proxy for region '%s', please check the config.", h.RegionSlug)
	}
	domain = endsWithDot(domain)

	cname := fmt.Sprintf("%s.%s", h.Slug, domain)
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

func (n *Navigator) find(slug string) (*Host, error) {
	h := new(Host)
	err := n.db.QueryRow(
		`SELECT hosts.id, hosts.slug, regions.id, regions.slug
		FROM hosts, regions WHERE hosts.slug = $1 AND hosts.region_id = regions.id LIMIT 1`, slug,
	).Scan(&h.Id, &h.Slug, &h.RegionID, &h.RegionSlug)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (n *Navigator) NewSOA() []dns.RR {
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

func New(domain string, config map[string]string) (nav.Navigator, error) {
	domainDot := endsWithDot(domain)
	db, err := sql.Open("postgres", config["database_url"])
	if err != nil {
		return nil, fmt.Errorf("Boxhost: failed to get db connection %v", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Boxhost: failed to get db connection %v", err)
	}
	return &Navigator{
		domain: domainDot,
		db:     db,
		config: config,
	}, nil
}

func endsWithDot(domain string) string {
	if strings.HasSuffix(domain, ".") {
		return domain
	} else {
		return fmt.Sprintf("%s.", domain)
	}
}

func init() {
	nav.Navigators["proxy"] = New
}
