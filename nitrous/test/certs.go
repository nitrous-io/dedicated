// this contains the helpers related to generating certificates
package test

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"

	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/envvars"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"
)

var (
	// default certPool for test. This will only be calculated once
	certPoolLock sync.Once
	certPool     *x509.CertPool
)

func init() {
	cert.KeyLen = 512
}

func TLSConfig(certPEM []byte, keyPEM []byte) (*tls.Config, error) {
	var err error
	certPoolLock.Do(func() {
		var caInfo *certauth.CAInfo
		caInfo, err = certauth.NewCAInfo(envvars.CACertPath, envvars.CAKeyPath)
		if err != nil {
			return
		}
		certPool = x509.NewCertPool()
		certPool.AddCert(caInfo.X509Cert)
	})
	if err != nil {
		return nil, err
	}

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		NextProtos:   []string{"http/1.1"},
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      certPool,
		ServerName:   fmt.Sprintf("*.%s", envvars.HostHostname),
	}, nil
}

// GenerateHostCert generates a certificate for the host
// signed by CA.  This is expensive, so call explicitly
// only when needed.  It deletes any existing cert for
// that host.
func GenerateHostCert(db *db.Database, hs *HostStack) (*HostStack, error) {
	caInfo, err := certauth.NewCAInfo(envvars.CACertPath, envvars.CAKeyPath)
	if err != nil {
		return nil, err
	}

	// in test, use a single hostname so we dont have to regenerate them
	hostname := fmt.Sprintf("*.%s", envvars.HostHostname)
	crt, err := cert.Generate(hostname, caInfo.X509Cert, caInfo.Key)
	if err != nil {
		return nil, err
	}
	tx, err := db.Beginx()
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := cert.Delete(tx, hs.H.ID); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := crt.Insert(tx, hs.H.ID); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	hs.Cert = crt
	return hs, nil
}

// These certs are the same as the certs embeded with net/http/httptest.
// If there's mismatched certs error, look at net/http/httptest/server.go to
// check if the cert has changed.
//
// localhostCert is a PEM-encoded TLS cert with SAN IPs
// "127.0.0.1" and "[::1]", expiring at the last second of 2049 (the end
// of ASN.1 time).
// generated from src/pkg/crypto/tls:
// go run generate_cert.go  --rsa-bits 512 --host 127.0.0.1,::1,example.com --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h
var localhostCert = `-----BEGIN CERTIFICATE-----
MIIBdzCCASOgAwIBAgIBADALBgkqhkiG9w0BAQUwEjEQMA4GA1UEChMHQWNtZSBD
bzAeFw03MDAxMDEwMDAwMDBaFw00OTEyMzEyMzU5NTlaMBIxEDAOBgNVBAoTB0Fj
bWUgQ28wWjALBgkqhkiG9w0BAQEDSwAwSAJBAN55NcYKZeInyTuhcCwFMhDHCmwa
IUSdtXdcbItRB/yfXGBhiex00IaLXQnSU+QZPRZWYqeTEbFSgihqi1PUDy8CAwEA
AaNoMGYwDgYDVR0PAQH/BAQDAgCkMBMGA1UdJQQMMAoGCCsGAQUFBwMBMA8GA1Ud
EwEB/wQFMAMBAf8wLgYDVR0RBCcwJYILZXhhbXBsZS5jb22HBH8AAAGHEAAAAAAA
AAAAAAAAAAAAAAEwCwYJKoZIhvcNAQEFA0EAAoQn/ytgqpiLcZu9XKbCJsJcvkgk
Se6AbGXgSlq+ZCEVo0qIwSgeBqmsJxUu7NCSOwVJLYNEBO2DtIxoYVk+MA==
-----END CERTIFICATE-----`

// localhostKey is the private key for localhostCert.
var localhostKey = `-----BEGIN RSA PRIVATE KEY-----
MIIBPAIBAAJBAN55NcYKZeInyTuhcCwFMhDHCmwaIUSdtXdcbItRB/yfXGBhiex0
0IaLXQnSU+QZPRZWYqeTEbFSgihqi1PUDy8CAwEAAQJBAQdUx66rfh8sYsgfdcvV
NoafYpnEcB5s4m/vSVe6SU7dCK6eYec9f9wpT353ljhDUHq3EbmE4foNzJngh35d
AekCIQDhRQG5Li0Wj8TM4obOnnXUXf1jRv0UkzE9AHWLG5q3AwIhAPzSjpYUDjVW
MCUXgckTpKCuGwbJk7424Nb8bLzf3kllAiA5mUBgjfr/WtFSJdWcPQ4Zt9KTMNKD
EUO0ukpTwEIl6wIhAMbGqZK3zAAFdq8DD2jPx+UJXnh0rnOkZBzDtJ6/iN69AiEA
1Aq8MJgTaYsDQWyU/hDq5YkDJc9e9DSCvUIzqxQWMQE=
-----END RSA PRIVATE KEY-----`
