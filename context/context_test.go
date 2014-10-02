package context_test

import (
	"errors"
	"io/ioutil"
	"testing"

	"github.com/nitrous-io/hostbuilder/context"
	"github.com/nitrous-io/hostbuilder/host"
	// Required since the Dummy provider needs to be init'ed in host.Hosts
	_ "github.com/nitrous-io/hostbuilder/host/dummy"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestContext(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "context")
}

var _ = Describe("context", func() {
	Describe("Execute", func() {

		Context("Successful Execution", func() {
			It("should run all steps in the context", func() {
				var i int

				increment := func(c *context.Context, h host.Host) error {
					i += 1
					return nil
				}
				h, err := host.InitHost("dummy", map[string]string{}, "")
				Expect(err).To(BeNil())
				steps := []context.Step{
					{"1st increment", increment},
					{"2nd increment", increment},
				}
				c := context.NewContext(h, steps, ioutil.Discard)
				c.Execute()

				Expect(i).To(Equal(len(steps)))
			})
		})

		Context("Unsuccessful Execution", func() {
			It("should fail and not execute all the steps", func() {
				var i int
				increment := func(c *context.Context, h host.Host) error {
					i += 1
					if i == 2 {
						return errors.New("oh it failed")
					}
					return nil
				}
				steps := []context.Step{
					{"1st increment", increment},
					{"2nd increment", increment},
					{"3rd increment", increment},
				}
				h, _ := host.InitHost("dummy", map[string]string{}, "")
				c := context.NewContext(h, steps, ioutil.Discard)
				err := c.Execute()
				Expect(err).ToNot(BeNil())
				Expect(c.Error).ToNot(BeNil())
				Expect(i).ToNot(Equal(len(steps)))
			})
		})
	})
})
