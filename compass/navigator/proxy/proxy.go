package proxy

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
	"github.com/miekg/dns"
	nav "github.com/nitrous-io/dedicated/compass/navigator"
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
	ProviderID int
	Provider   string
}

type Navigator struct {
	domain string
	db     *sql.DB
	config map[string]string
}

func (n *Navigator) Name() string {
	return "proxy"
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
	domain = endsWithDot(domain)

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

func (n *Navigator) find(slug string) (*Host, error) {
	h := new(Host)
	err := n.db.QueryRow(
		`SELECT hosts.id, hosts.slug, regions.id, regions.slug, provider_templates.id, provider_templates.provider
		FROM hosts, regions, provider_templates WHERE hosts.slug = $1 AND hosts.region_id = regions.id
		AND hosts.provider_template_id = provider_templates.id LIMIT 1`, slug,
	).Scan(&h.Id, &h.Slug, &h.RegionID, &h.RegionSlug, &h.ProviderID, &h.Provider)
	if err != nil {
		return nil, err
	}
	return h, nil
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
