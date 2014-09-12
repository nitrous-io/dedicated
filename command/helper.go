package command

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nitrous-io/nitro/consts"
)

var (
	ErrUnknown = fmt.Errorf("Unknown error occurred")
)

// CheckResponse retrieves the error_description on error responses
func CheckResponse(resp *http.Response, expectedCode int) error {
	if resp.StatusCode == expectedCode {
		return nil
	}
	j := map[string]interface{}{}
	err := json.NewDecoder(resp.Body).Decode(&j)
	if err != nil {
		return ErrUnknown
	}
	errDesc, ok := j["error_description"].(string)
	if !ok {
		return ErrUnknown
	}
	return fmt.Errorf(errDesc)
}

// CertPool returns a x509.CertPool that contains the root ca
func CACertPool() (*x509.CertPool, error) {
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(consts.CACert); !ok {
		return nil, fmt.Errorf("Could not add cert")
	}
	return certPool, nil
}

// ClientConfig returns the necessary TLS config to connect to hostname
func ClientConfig(hostname string) (*tls.Config, error) {
	pool, err := CACertPool()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		ServerName: hostname,
		RootCAs:    pool,
	}, nil
}
