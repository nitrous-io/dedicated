package context_test

import (
	"testing"

	// Required since the Dummy provider needs to be init'ed in host.Hosts
	_ "github.com/nitrous-io/dedicated/hostbuilder/host/dummy"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestContext(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "context")
}

var _ = Describe("context", func() {
	Describe("Run", func() {
		It("runs the given step within its timeout", func() {
			// FIXME Add tests.
		})
	})
})
