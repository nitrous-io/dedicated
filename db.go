package tender

import (
	"database/sql"
	"net"
)

const (
	// get the host
	hostQuery = `
	SELECT hosts.ip_address
	FROM hosts
	WHERE hosts.slug = $1
	`
)

// hostIP returns ip address of a host given its unique slug
func hostIP(db *sql.DB, slug string) (net.IP, error) {
	var ip string
	err := db.QueryRow(hostQuery, slug).Scan(&ip)
	if err != nil {
		return nil, err
	}
	return net.ParseIP(ip), nil
}
