package tender

import (
	"database/sql"
	"net"
)

const (
	// get the user id if we know the access token of that user
	userQuery = `
	SELECT users.id
	FROM users INNER JOIN tokens ON tokens.user_id = users.id
	WHERE tokens.token = $1 
	`

	// get the host
	hostQuery = `
	SELECT hosts.ip_address
	FROM hosts
	WHERE hosts.name = $1
	`

	// get the host id and ip address if we know the user and the host name
	hostUserQuery = `
	SELECT hosts.ip_address
	FROM hosts INNER JOIN users ON hosts.user_id = users.id
	WHERE users.id = $1 AND hosts.name = $2
	`
)

func user(db *sql.DB, token string) (int, error) {
	var id int
	err := db.QueryRow(userQuery, token).Scan(&id)
	return id, err
}

func host(db *sql.DB, hostName string) (net.IP, error) {
	var ip string
	err := db.QueryRow(hostQuery, hostName).Scan(&ip)
	if err != nil {
		return nil, err
	}
	return net.ParseIP(ip), nil
}

func hostByUser(db *sql.DB, userID int, hostName string) (net.IP, error) {
	var ip string
	err := db.QueryRow(hostUserQuery, userID, hostName).Scan(&ip)
	if err != nil {
		return nil, err
	}
	return net.ParseIP(ip), nil
}
