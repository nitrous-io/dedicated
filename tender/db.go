package tender

import (
	"database/sql"
	"log"
	"net"
)

// Host describes a host machine
type Host struct {
	IP   net.IP
	Port int
	Slug string
}

// Container describes a container
type Container struct {
	CID        string
	Slug       string
	HostSlug   string
	HostIP     net.IP
	HostPort   int
	HostSecret string
}

// HostsMap is a map from slug to Host
type HostsMap map[string]*Host

// ContainersMap is a map from container slug to Container
type ContainersMap map[string]*Container

// findHost returns the host data given its unique slug
func findHost(db *sql.DB, slug string) (*Host, error) {
	var ip string
	var port int
	err := db.QueryRow(`
		SELECT hosts.ip_address, hosts.freighterd_port
		FROM hosts
		WHERE hosts.slug = $1 AND hosts.terminated_at IS NULL
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

func getHosts(db *sql.DB) (HostsMap, error) {
	m := HostsMap{}
	// TODO: should log time it takes to do this query so that we can have an
	// alert if it takes too long (eg. more than 5 secs)
	rows, err := db.Query(`
			SELECT slug, ip_address, freighterd_port
			FROM hosts
			WHERE terminated_at IS NULL
			AND state = 'running'
			AND COALESCE(ip_address, '') != ''
		`)
	if err != nil {
		log.Printf("[ERROR] db.Query() failed with '%s'", err)
		return nil, err
	}
	for rows.Next() {
		var slug, ip string
		var port int
		err = rows.Scan(&slug, &ip, &port)
		if err != nil {
			log.Printf("[ERROR] rows.Scan() failed with '%s'", err)
			rows.Close()
			return nil, err
		}
		h := &Host{
			Slug: slug,
			IP:   net.ParseIP(ip),
			Port: port,
		}
		m[slug] = h
	}
	err = rows.Close()
	if err != nil {
		log.Printf("[ERROR] rows.Close() failed with '%s'", err)
		return nil, err
	}
	return m, nil
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

func getContainers(db *sql.DB) (ContainersMap, error) {
	m := ContainersMap{}
	rows, err := db.Query(`
		SELECT containers.slug, containers.cid, hosts.slug, hosts.ip_address, hosts.freighterd_port, hosts.secret
		FROM containers INNER JOIN hosts ON containers.host_id = hosts.id
		WHERE containers.terminated_at IS NULL
	`)
	if err != nil {
		log.Printf("[ERROR] db.Query() failed with '%s'", err)
		return nil, err
	}

	for rows.Next() {
		var slug, cid, ip, hostSlug, secret string
		var port int
		err = rows.Scan(&slug, &cid, &hostSlug, &ip, &port, &secret)
		if err != nil {
			return nil, err
		}
		c := &Container{
			CID:        cid,
			Slug:       slug,
			HostSlug:   hostSlug,
			HostIP:     net.ParseIP(ip),
			HostPort:   port,
			HostSecret: secret,
		}
		m[slug] = c
	}
	err = rows.Close()
	if err != nil {
		log.Printf("[ERROR] rows.Close() failed with '%s'", err)
		return nil, err
	}
	return m, nil
}
