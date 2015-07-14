package test

import (
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	"github.com/nitrous-io/dedicated/nitrous/controllers"
	"github.com/nitrous-io/dedicated/nitrous/db"
	"github.com/nitrous-io/dedicated/nitrous/models/cert"
	"github.com/nitrous-io/dedicated/nitrous/models/host"
)

type freighterClientFn func(db db.QueryReceiver, h *host.Host, uuid string) (*controllers.FreighterClient, error)
type freighterClientWithCertFn func(cert *cert.Cert, h *host.Host, uuid string) (*controllers.FreighterClient, error)

var actualClient = controllers.NewFreighterClient
var actualClientWithCert = controllers.NewFreighterClientWithCert

// SetDefaultFreighterFn assumes that we only want to inject the client, and we
// have a valid host with proper cert.
func StubDefaultFreighterFn(cl freighterclient.Client) {
	controllers.NewFreighterClient = func(db db.QueryReceiver, h *host.Host, uuid string) (*controllers.FreighterClient, error) {
		crt, err := cert.Find(db, h.ID)
		if err != nil {
			return nil, err
		}
		return &controllers.FreighterClient{
			Client: cl,
			Cert:   crt,
		}, nil
	}

	controllers.NewFreighterClientWithCert = func(cert *cert.Cert, h *host.Host, uuid string) (*controllers.FreighterClient, error) {
		return &controllers.FreighterClient{
			Client: cl,
			Cert:   crt,
		}, nil
	}
}

func UnstubFreighterFn() {
	controllers.NewFreighterClient = actualClient
	controllers.NewFreighterClientWithCert = actualClientWithCert
}
