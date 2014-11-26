package container

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	_ "github.com/lib/pq"
	"github.com/miekg/dns"
	nav "github.com/nitrous-io/dedicated/compass/navigator"
)

const defaultTTL = uint32(300)

var (
	ErrNotFound = errors.New("RecordNotFound")
)

type Container struct {
	Id     int
	Slug   string
	HostIP string
}

type Navigator struct {
	domain string
	db     *sql.DB
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
		return nil, fmt.Errorf("missing box name %v", name)
	}
	slug := labels[llabel-3]
	c, err := n.find(slug)
	if err != nil {
		return nil, fmt.Errorf("Invalid boxname from domain %v", name)
	}
	return []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    defaultTTL,
			},
			A: net.ParseIP(c.HostIP),
		},
	}, nil
}

func (n *Navigator) find(slug string) (*Container, error) {
	c := new(Container)
	err := n.db.QueryRow(`SELECT containers.id, containers.slug, hosts.ip_address
	FROM containers INNER JOIN hosts ON containers.host_id = hosts.id
	WHERE containers.slug = $1 LIMIT 1`, slug).Scan(&c.Id, &c.Slug, &c.HostIP)
	if err != nil {
		return nil, err
	}
	return c, nil
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
	}, nil
}

func init() {
	nav.Navigators["container"] = New
}
