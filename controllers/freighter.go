package controllers

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"code.google.com/p/go.crypto/ssh"

	fc "github.com/nitrous-io/freighter/freighterclient"
	"github.com/nitrous-io/nitrous/db"
	"github.com/nitrous-io/nitrous/models/cert"
	"github.com/nitrous-io/nitrous/models/host"
	"github.com/nitrous-io/nitrous/pkg/httperror"
)

var (
	// ErrHostIPUnknown results when the host's ip address is empty, hence not
	// usable by the client
	ErrHostIPUnknown = httperror.New(http.StatusBadRequest, "invalid_request", "host IP is unknown")
	// ErrHostNotRunning results when we tries to access a not-running host
	ErrHostNotRunning = httperror.New(http.StatusServiceUnavailable, "service_unavailable", "host is not running")
)

var caCertPath, caKeyPath string

func init() {
	caKeyPath = os.Getenv("CA_KEY_PATH")
	if caKeyPath == "" {
		panic("CA_KEY_PATH is not set")
	}
	caCertPath = os.Getenv("CA_CERT_PATH")
	if caCertPath == "" {
		panic("CA_CERT_PATH is not set")
	}

	// ensure CA is valid on start
	if _, err := NewCAInfo(caCertPath, caKeyPath); err != nil {
		panic(fmt.Errorf("Error with keypair (%s,%s): %s", caCertPath, caKeyPath, err))
	}
}

// FreighterClient embeds freighterclient.Client, but also expose the used
// certificate for HTTPS connection
type FreighterClient struct {
	fc.Client
	Cert *cert.Cert
}

// NewFreighterClient returns a client that is able to communicate with a
// freighter server using HTTPS
var NewFreighterClient = func(db db.QueryReceiver, h *host.Host) (*FreighterClient, error) {
	if h.IPAddress.String == "" {
		return nil, ErrHostIPUnknown
	}
	if h.State != host.StateRunning {
		return nil, ErrHostNotRunning
	}
	crt, err := cert.Find(db, h.ID)
	if err != nil {
		return nil, err
	}
	caInfo, err := GetCAInfo(false)
	if err != nil {
		return nil, err
	}
	hst := fc.NewHost(
		h.IPAddress.String,
		int(h.FreighterdPort.Int64),
		h.Hostname(),
		caInfo.CertPEM,
	)
	cl, err := fc.NewClient(hst, h.Secret)
	if err != nil {
		return nil, err
	}
	return &FreighterClient{
		Client: cl,
		Cert:   crt,
	}, nil
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
