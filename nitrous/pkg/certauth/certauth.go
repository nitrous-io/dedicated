package certauth

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"

	"github.com/nitrous-io/dedicated/nitrous/envvars"

	"golang.org/x/crypto/ssh"
)

var caCertPath, caKeyPath string

// FIXME Move the env var assumptions and panics to callers where necessary.
// These don't belong in a library.
func init() {
	caKeyPath = envvars.CAKeyPath
	if caKeyPath == "" {
		panic("CA_KEY_PATH is not set")
	}
	caCertPath = envvars.CACertPath
	if caCertPath == "" {
		panic("CA_CERT_PATH is not set")
	}

	// ensure CA is valid on start
	if _, err := NewCAInfo(caCertPath, caKeyPath); err != nil {
		panic(fmt.Errorf("Error with keypair (%s,%s): %s", caCertPath, caKeyPath, err))
	}
}

// CACertInfo contains various representations of a CA
type CAInfo struct {
	CertPEM  []byte
	KeyPEM   []byte
	X509Cert *x509.Certificate
	TLSCert  tls.Certificate
	Key      *rsa.PrivateKey
}

// GetCAInfo returns a populated CAInfo with root CA.  If
// full is false, only populates CertPEM and X509Cert
// (fields that do not require reading the key)
func GetCAInfo(full bool) (*CAInfo, error) {
	kp := ""
	if full {
		kp = caKeyPath
	}
	return NewCAInfo(caCertPath, kp)
}

// NewCAInfo returns a populated CAInfo struct given paths to the cert and key
// PEMs.  If keyPath is empty, it will only populate CertPEM and X509Cert.
func NewCAInfo(certPath, keyPath string) (*CAInfo, error) {
	info := &CAInfo{}
	bs, err := ioutil.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	info.CertPEM = bs

	block, _ := pem.Decode(info.CertPEM)
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	info.X509Cert = x509Cert

	if keyPath == "" {
		return info, nil
	}

	bs, err = ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	info.KeyPEM = bs

	key, err := ssh.ParseRawPrivateKey(info.KeyPEM)
	if err != nil {
		return nil, err
	}
	info.Key = key.(*rsa.PrivateKey)

	tlsCert, err := tls.X509KeyPair(info.CertPEM, info.KeyPEM)
	if err != nil {
		return nil, err
	}
	info.TLSCert = tlsCert

	return info, nil
}
