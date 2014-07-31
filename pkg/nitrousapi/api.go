package nitrousapi

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nitrous-io/nitrous-common/httpclient"
)

type ContainerAPI struct {
	ID        int64  `json:"id"`
	CID       string `json:"cid"`
	UserID    int64  `json:"user_id"`
	Image     string `json:"image"`
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	State     string `json:"state"`

	Host HostAPI `json:"host"`
}

type HostAPI struct {
	ID             int64  `json:"id"`
	Slug           string `json:"slug"`
	IPAddress      string `json:"ip_address"`
	FreighterdPort int64  `json:"freighterd_port"`
	Domain         string `json:"domain"`

	CA   string `json:"ca"`
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

type containerResp struct {
	Container *ContainerAPI `json:"container"`
}

func GetContainer(client *httpclient.Client, server, slug string) (*ContainerAPI, error) {
	// get container data
	endpoint := fmt.Sprintf("%s/api/v0/containers/%s", server, slug)
	resp, err := client.GetAuthed(endpoint, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Failed to find container %s", slug)
	}

	cr := &containerResp{}
	if err := json.NewDecoder(resp.Body).Decode(cr); err != nil {
		return nil, err
	}
	return cr.Container, nil
}

// TLSConfig returns a tls configuration generated from api response that is
// valid for establishing HTTPS connection with the dedicated host
func TLSConfig(h HostAPI) (*tls.Config, error) {
	crt, err := tls.X509KeyPair([]byte(h.Cert), []byte(h.Key))
	if err != nil {
		return nil, err
	}
	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM([]byte(h.CA))
	tlsConfig := &tls.Config{
		ServerName:   h.Domain,
		RootCAs:      certPool,
		Certificates: []tls.Certificate{crt},
	}
	return tlsConfig, nil
}
