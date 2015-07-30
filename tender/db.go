package tender

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net"

	"github.com/Sirupsen/logrus"
)

// Host describes a host machine
type Host struct {
	IP   net.IP
	Port int
	Slug string
}

// Container describes a container
type Container struct {
	CID          string
	Slug         string
	HostSlug     string
	HostIP       net.IP
	HostPort     int
	HostSecret   string
	PortMappings map[string]string
}

// HostsMap is a map from slug to Host
type HostsMap map[string]*Host

// ContainersMap is a map from container slug to Container
type ContainersMap map[string]*Container

// findHost returns the host data given its unique slug
func findHost(db *sql.DB, slug string) (*Host, error) {
	var ip sql.NullString
	var port int
	err := db.QueryRow(`
		SELECT ip_address, freighterd_port
		FROM hosts
		WHERE slug = $1
		  AND terminated_at IS NULL
		  AND state = 'running'
		  AND COALESCE(ip_address, '') != ''
	`, slug).Scan(&ip, &port)
	if err != nil {
		return nil, err
	}
	if !ip.Valid {
		return nil, errors.New("ip_address is NULL")
	}
	return &Host{
		Slug: slug,
		IP:   net.ParseIP(ip.String),
		Port: port,
	}, nil
}

func getHosts(db *sql.DB) (HostsMap, error) {
	m := HostsMap{}
	q := `
		SELECT slug, ip_address, freighterd_port
		FROM hosts
		WHERE terminated_at IS NULL
		AND state = 'running'
		AND COALESCE(ip_address, '') != ''
	`
	rows, err := db.Query(q)
	if err != nil {
		logrus.Errorf("db.Query('%s') failed with '%s'", q, err)
		return nil, err
	}
	for rows.Next() {
		var slug, ip sql.NullString
		var port int
		err = rows.Scan(&slug, &ip, &port)
		if err != nil {
			logrus.Errorf("rows.Scan() failed with '%s'", err)
			rows.Close()
			return nil, err
		}
		if !ip.Valid {
			logrus.Errorf("ip_address is NULL")
			continue
		}
		if !slug.Valid {
			logrus.Errorf("slug is NULL")
			continue
		}
		h := &Host{
			Slug: slug.String,
			IP:   net.ParseIP(ip.String),
			Port: port,
		}
		m[slug.String] = h
	}
	err = rows.Close()
	if err != nil {
		logrus.Errorf("rows.Close() failed with '%s'", err)
		return nil, err
	}
	return m, nil
}

// findContainer returns the container data given its unique slug
func findContainer(db *sql.DB, slug string) (*Container, error) {
	var cid, ip, hostSlug, secret, portMappingsJson string
	var port int
	err := db.QueryRow(`
		SELECT containers.cid, hosts.slug, hosts.ip_address, hosts.freighterd_port, hosts.secret, containers.port_mappings
		FROM containers INNER JOIN hosts ON containers.host_id = hosts.id
		WHERE containers.slug = $1 AND containers.terminated_at IS NULL
	`, slug).Scan(&cid, &hostSlug, &ip, &port, &secret, &portMappingsJson)
	if err != nil {
		return nil, err
	}

	var portMappings map[string]string
	err = json.Unmarshal([]byte(portMappingsJson), &portMappings)
	if err != nil {
		return nil, err
	}

	return &Container{
		CID:          cid,
		Slug:         slug,
		HostSlug:     hostSlug,
		HostIP:       net.ParseIP(ip),
		HostPort:     port,
		HostSecret:   secret,
		PortMappings: portMappings,
	}, nil
}

func getContainers(db *sql.DB) (ContainersMap, error) {
	m := ContainersMap{}
	q := `SELECT containers.slug, containers.cid, hosts.slug, hosts.ip_address, hosts.freighterd_port, hosts.secret, containers.port_mappings
		FROM containers INNER JOIN hosts ON containers.host_id = hosts.id
		WHERE containers.terminated_at IS NULL AND
			hosts.state = 'running'`
	rows, err := db.Query(q)
	if err != nil {
		logrus.Errorf("db.Query('%s') failed with '%s'", q, err)
		return nil, err
	}

	for rows.Next() {
		var slug, cid, ip, hostSlug, secret, portMappingsJson string
		var port int
		err = rows.Scan(&slug, &cid, &hostSlug, &ip, &port, &secret, &portMappingsJson)
		if err != nil {
			return nil, err
		}

		var portMappings map[string]string
		err = json.Unmarshal([]byte(portMappingsJson), &portMappings)
		if err != nil {
			return nil, err
		}

		c := &Container{
			CID:          cid,
			Slug:         slug,
			HostSlug:     hostSlug,
			HostIP:       net.ParseIP(ip),
			HostPort:     port,
			HostSecret:   secret,
			PortMappings: portMappings,
		}
		m[slug] = c
	}
	err = rows.Close()
	if err != nil {
		logrus.Errorf("rows.Close() failed with '%s'", err)
		return nil, err
	}
	return m, nil
}
