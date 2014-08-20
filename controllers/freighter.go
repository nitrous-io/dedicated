package controllers

import (
	"crypto/rsa"
	"io/ioutil"
	"net/http"
	"os"

	"code.google.com/p/go.crypto/ssh"
	"github.com/nitrous-io/sslider/sslgen"

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
	ca, err := CACertPEM()
	if err != nil {
		return nil, err
	}
	hst := fc.NewHost(
		h.IPAddress.String,
		int(h.FreighterdPort.Int64),
		h.Hostname(),
		ca, // TODO this can go away once freighter is updated to not require the argument
	)
	cl, err := fc.NewClient(hst)
	if err != nil {
		return nil, err
	}
	return &FreighterClient{
		Client: cl,
		Cert:   crt,
	}, nil
}

func CACert() (*sslgen.Cert, error) {
	return certFromKey(caKeyPath)
}

func CACertPEM() ([]byte, error) {
	return ioutil.ReadFile(caCertPath)
}

// TODO: move into sslgen?
func certFromKey(keyPath string) (*sslgen.Cert, error) {
	c := &sslgen.Cert{}
	privBytes, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	priv, err := ssh.ParseRawPrivateKey(privBytes)
	if err != nil {
		return nil, err
	}
	pk := priv.(*rsa.PrivateKey)

	c.PrivateKey = pk
	c.PublicKey = (*rsa.PublicKey)(&pk.PublicKey)
	return c, nil
}
