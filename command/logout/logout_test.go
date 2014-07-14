package logout_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	. "github.com/nitrous-io/nitro/command/logout"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestLogout(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "logout")
}

var _ = Describe("logout", func() {
	var (
		cmd  *Command
		ui   *cli.MockUi
		args []string

		testConfigFile *os.File
	)

	BeforeEach(func() {
		ui = &cli.MockUi{}
		cmd = &Command{UI: ui, Server: "www.example.com"}

		f, err := ioutil.TempFile("", "nitrocfg")
		Expect(err).To(BeNil())
		testConfigFile = f
		command.Env.ConfigPath = f.Name()
	})

	AfterEach(func() {
		testConfigFile.Close()
		os.Remove(testConfigFile.Name())
	})

	It("implements cli.Command", func() {
		var _ cli.Command = cmd
	})

	Context("when token exist in config file", func() {
		BeforeEach(func() {
			c := &command.ConfigFile{Auths: make(map[string]command.AuthConfig)}
			a := command.AuthConfig{}
			a.Username = "foo"
			a.Token = "bar"
			c.Auths["www.example.com"] = a

			command.SaveConfig(c)
		})

		AfterEach(func() {
			err := os.Remove(testConfigFile.Name())
			Expect(err).To(BeNil())
		})

		It("removes the token", func() {
			exitCode := cmd.Run(args)
			Expect(exitCode).To(Equal(0))

			c, err := command.LoadConfig()
			Expect(err).To(BeNil())

			Expect(c.Auths["www.example.com"].Username).To(BeEmpty())
			Expect(c.Auths["www.example.com"].Token).To(BeEmpty())
		})
	})
})
