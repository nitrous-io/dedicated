package provisioner

import (
	"crypto/rsa"

	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"
	"github.com/nitrous-io/dedicated/nitrous/pkg/hostbuilderclient"
)

func GenerateKey() (priv *rsa.PrivateKey, pub []byte, err error) {
	return generateKey()
}

func (p *Provisioner) MakeHostbuilderConfig(phost *preparedHost, opts ProvisionOptions) (hostbuilderclient.Config, error) {
	return p.mkHostbuilderConfig(phost, opts)
}

func NewPreparedHost(h *host.Host) *preparedHost {
	return &preparedHost{
		host:   h,
		caInfo: &certauth.CAInfo{},
		cert:   &cert.Cert{},
	}
}
