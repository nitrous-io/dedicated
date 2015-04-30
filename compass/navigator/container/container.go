package container

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

type Container struct {
	Id         int
	Slug       string
	RegionID   int
	RegionSlug string
	Provider   string
}

type Navigator struct {
	domain string
	db     *sql.DB
	config map[string]string
}

func (n *Navigator) Name() string {
	return "container"
}

func (n *Navigator) Lookup(name string) ([]dns.RR, error) {
	if !nav.SameRootDomain(name, n.domain) {
		return nil, fmt.Errorf("cant handle domain %v", name)
	}
	labels := dns.SplitDomainName(name)
	llabel := len(labels)
	if len(labels) < 3 {
		return nil, fmt.Errorf("Missing container slug %v", name)
	}

	slug := labels[llabel-3]
	// strings.ToLower is safe here because we expect name to have been
	// converted to Punycode when necessary.
	c, err := n.find(strings.ToLower(slug))
	if err != nil {
		return nil, fmt.Errorf("Invalid container slug from domain %v", name)
	}

	domain := n.config[c.RegionSlug]
	if len(domain) == 0 {
		return nil, fmt.Errorf("Empty proxy for region '%s', please check the config.", c.RegionSlug)
	}
	domain = endsWithDot(domain)

	cname := fmt.Sprintf("%s.%s", c.Provider, domain)
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

func (n *Navigator) find(slug string) (*Container, error) {
	c := new(Container)
	err := n.db.QueryRow(
		`SELECT containers.id, containers.slug, regions.id, regions.slug, provider_templates.provider
		FROM containers
		INNER JOIN hosts ON containers.host_id = hosts.id
		INNER JOIN regions ON hosts.region_id = regions.id
		INNER JOIN provider_templates ON hosts.provider_template_id = provider_templates.id
		WHERE containers.slug = $1 LIMIT 1`, slug).
		Scan(&c.Id, &c.Slug, &c.RegionID, &c.RegionSlug, &c.Provider)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func New(domain string, config map[string]string) (nav.Navigator, error) {
	db, err := sql.Open("postgres", config["database_url"])
	if err != nil {
		return nil, fmt.Errorf("Box: failed to get db connection %v", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Box: failed to ping db %v", err)
	}
	return &Navigator{
		domain: domain,
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
	nav.Navigators["container"] = New
}
