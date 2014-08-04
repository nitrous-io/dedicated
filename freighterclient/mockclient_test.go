package freighterclient_test

import (
	"github.com/nitrous-io/freighter/freighterclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// we use the actual freighterclient in client tests; this is to ensure that
// MockClient is not broken for external users
var _ = Describe("mockclient", func() {
	It("implements freighterclient.Client", func() {
		var _ freighterclient.Client = &freighterclient.MockClient{}
	})

	It("returns docker config", func() {
		mc := &freighterclient.MockClient{}
		dcc, err := mc.DockerContainerConfig()
		Expect(err).To(BeNil())
		Expect(dcc).To(BeNil())

		dcc = freighterclient.NewDockerContainerConfig()
		dcc.Env = []string{"LANG=C"}
		mc.Create("stub", "stub", dcc)
		mockConf, err := mc.DockerContainerConfig()
		Expect(err).To(BeNil())
		Expect(mockConf).NotTo(BeNil())
		Expect(mockConf.Env).To(Equal(dcc.Env))
	})

	// TODO flesh out mockclient tests
})
