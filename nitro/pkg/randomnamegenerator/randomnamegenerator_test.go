package randomnamegenerator_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/nitrous-io/dedicated/nitro/pkg/randomnamegenerator"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "randomnamegenerator")
}

var _ = Describe("randomnamegenerator", func() {
	It("generates a random name with the given suffix", func() {
		name := randomnamegenerator.Generate("foobar")
		// This checks whether there is an adjective and a noun (both of which can contain further hyphens)
		Expect(name).To(MatchRegexp(`^(\w+-?){2,}-foobar-\d+$`))
	})

	It("differs when called a second time", func() {
		Expect(randomnamegenerator.Generate("foobar")).NotTo(Equal(randomnamegenerator.Generate("foobar")))
	})
})
