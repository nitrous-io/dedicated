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

func getHosts(db *sql.DB) (HostsMap, error) {
	m := HostsMap{}
	// TODO: should log time it takes to do this query so that we can have an
	// alert if it takes too long (eg. more than 5 secs)
	rows, err := db.Query(`
			SELECT slug, ip_address, freighterd_port
			FROM hosts
			WHERE hosts.terminated_at IS NULL
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
