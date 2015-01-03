package certauth

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io/ioutil"

	"golang.org/x/crypto/ssh"
)

// CACertInfo contains various representations of a CA
type CAInfo struct {
	CertPEM  []byte
	KeyPEM   []byte
	X509Cert *x509.Certificate
	TLSCert  tls.Certificate
	Key      *rsa.PrivateKey
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
