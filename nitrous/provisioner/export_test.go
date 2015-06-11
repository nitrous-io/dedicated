package provisioner

import (
	"crypto/rsa"

	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
	"github.com/nitrous-io/dedicated/nitrous/pkg/certauth"
)

func GenerateKey() (priv *rsa.PrivateKey, pub []byte, err error) {
	return generateKey()
}

func NewPreparedHost(h *host.Host) *preparedHost {
	return &preparedHost{
		host:   h,
		caInfo: &certauth.CAInfo{},
		cert:   &cert.Cert{},
	}
}
