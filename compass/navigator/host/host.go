package host

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
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
	Id        int
	Slug      string
	IpAddress string
}

type Navigator struct {
	domain string
	db     *sql.DB
}

func (n *Navigator) Name() string {
	return "host"
}

func (n *Navigator) Lookup(name string) ([]dns.RR, error) {
	if !nav.SameRootDomain(name, n.domain) {
		return nil, fmt.Errorf("cant handle domain %v", name)
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

func (n *Navigator) find(slug string) (*Host, error) {
	h := new(Host)
	err := n.db.QueryRow(
		"SELECT id, slug, ip_address FROM hosts WHERE slug = $1 LIMIT 1",
		slug).Scan(&h.Id, &h.Slug, &h.IpAddress)
	if err != nil {
		return nil, err
	}
	return h, nil
}

func New(domain string, config map[string]string) (nav.Navigator, error) {
	db, err := sql.Open("postgres", config["database_url"])
	if err != nil {
		return nil, fmt.Errorf("Boxhost: failed to get db connection %v", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Boxhost: failed to get db connection %v", err)
	}
	return &Navigator{
		domain: domain,
		db:     db,
	}, nil
}

func init() {
	nav.Navigators["host"] = New
}
