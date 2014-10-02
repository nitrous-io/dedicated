package freighterclient_test

import (
	"github.com/nitrous-io/dedicated/freighter/freighterclient"
	. "github.com/onsi/ginkgo"
)

// we use the actual freighterclient in client tests; this is to ensure that
// MockClient is not broken for external users
var _ = Describe("mockclient", func() {
	It("implements freighterclient.Client", func() {
		var _ freighterclient.Client = &freighterclient.MockClient{}
	})
})
