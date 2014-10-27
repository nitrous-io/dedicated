package tender

import (
	"database/sql"
	"net"
)

const (
	// get the host
	hostQuery = `
	SELECT hosts.ip_address, hosts.freighterd_port
	FROM hosts
	WHERE hosts.slug = $1
	`
)

// hostIP returns ip address and port number of a host given its unique slug
func hostIP(db *sql.DB, slug string) (net.IP, int, error) {
	var ip string
	var port int
	err := db.QueryRow(hostQuery, slug).Scan(&ip, &port)
	if err != nil {
		return nil, 0, err
	}
	return net.ParseIP(ip), port, nil
}
