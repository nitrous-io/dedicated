package cert

import (
	"crypto/rsa"
	"crypto/x509"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nitrous-io/dedicated/nitrous/db"
)

type Cert struct {
	ID     int64 `db:"id"`
	HostID int64 `db:"host_id"`

	ServerCert string `db:"server_cert"`
	ServerKey  string `db:"server_key"`

	TerminatedAt *time.Time `db:"terminated_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`
}

// Insert creates a new entry for cert in database. We specifically use
// `*sql.Tx` becase we requires the cert to be inserted in the same transaction
// as the host
func (c *Cert) Insert(tx *sqlx.Tx, hostID int64) error {
	c.HostID = hostID
	stmt, err := tx.PrepareNamed(`INSERT INTO certs
		(host_id, server_cert, server_key)
		VALUES (:host_id, :server_cert, :server_key)
		RETURNING *;`)
	if err != nil {
		return err
	}
	return stmt.QueryRowx(c).StructScan(c)
}

// Find gets the cert belongs to a host from database
func Find(db db.QueryReceiver, hostID int64) (*Cert, error) {
	crt := &Cert{}
	q := `SELECT * FROM certs WHERE host_id = $1 AND terminated_at IS NULL`
	err := db.Get(crt, q, hostID)
	if err != nil {
		return nil, err
	}
	return crt, nil
}

// Find gets the cert belongs to hosts from database
func FindAllByHostIDs(db db.QueryReceiver, hostIDs []int64) ([]*Cert, error) {
	crts := []*Cert{}

	q, args, err := sqlx.In("SELECT * FROM certs WHERE host_id IN(?) AND terminated_at IS NULL;", hostIDs)
	if err != nil {
		return nil, err
	}

	q = db.Rebind(q)
	err = db.Select(&crts, q, args...)
	if err != nil {
		return nil, err
	}
	return crts, nil
}

// Delete terminates the cert belongs to a host in database. We specifically use
// `*sql.Tx` becase we requires the cert to be deleted in the same transaction
// as the host
func Delete(tx *sqlx.Tx, hostID int64) error {
	_, err := tx.Exec("UPDATE certs SET terminated_at = now() WHERE host_id = $1", hostID)
	return err
}

// generate cert with server certificate and key. The
// cert will be used for TLS encryption
func Generate(hostname string,
	caCert *x509.Certificate,
	caKey *rsa.PrivateKey,
) (*Cert, error) {
	srvCrt, srvKey, err := GenerateHostCert(hostname, caCert, caKey)
	if err != nil {
		return nil, err
	}

	return &Cert{
		ServerCert: string(srvCrt),
		ServerKey:  string(srvKey),
	}, nil
}
