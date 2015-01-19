package tender

import (
	"database/sql"
	"net"
)

type Host struct {
	IP   net.IP
	Port int
	Slug string
}

// findHost returns the host data given its unique slug
func findHost(db *sql.DB, slug string) (*Host, error) {
	var ip string
	var port int
	err := db.QueryRow(`
		SELECT hosts.ip_address, hosts.freighterd_port
		FROM hosts
		WHERE hosts.slug = $1
	`, slug).Scan(&ip, &port)
	if err != nil {
		return nil, err
	}
	return &Host{
		Slug: slug,
		IP:   net.ParseIP(ip),
		Port: port,
	}, nil
}

type Container struct {
	CID        string
	Slug       string
	HostSlug   string
	HostIP     net.IP
	HostPort   int
	HostSecret string
}

// findContainer returns the container data given its unique slug
func findContainer(db *sql.DB, slug string) (*Container, error) {
	var cid, ip, hostSlug, secret string
	var port int
	err := db.QueryRow(`
		SELECT containers.cid, hosts.slug, hosts.ip_address, hosts.freighterd_port, hosts.secret
		FROM containers INNER JOIN hosts ON containers.host_id = hosts.id
		WHERE containers.slug = $1 AND containers.terminated_at IS NULL
	`, slug).Scan(&cid, &hostSlug, &ip, &port, &secret)
	if err != nil {
		return nil, err
	}
	return &Container{
		CID:        cid,
		Slug:       slug,
		HostSlug:   hostSlug,
		HostIP:     net.ParseIP(ip),
		HostPort:   port,
		HostSecret: secret,
	}, nil
}
