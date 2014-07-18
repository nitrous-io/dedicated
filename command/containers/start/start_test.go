package start_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/containers/start"
	"github.com/nitrous-io/nitro/test"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "containers/start")
}

var _ = Describe("containers/start", func() {
	var (
		cmd        *start.Command
		ui         *cli.MockUi
		configPath string

		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string
	)

	BeforeEach(func() {
		var err error
		ui = &cli.MockUi{}

		handlers = ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("POST", "/api/v0/hosts/host-1/containers/gregarious-scribe-24/start"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &start.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}

		configPath, err = test.WriteConfig(&command.ConfigFile{}, "")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		os.Remove(configPath)
	})

	Context("nitro start hostname/containername", func() {
		doCmd := func() int {
			return cmd.Run([]string{"host-1/gregarious-scribe-24"})
		}

		Context("when the API responds with the container", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `{ "container": { "name": "gregarious-scribe-24", "image": "ubuntu", "state": "running" } }`
			})

			It("prints the container", func() {
				code := doCmd()
				output := ui.OutputWriter.String()
				Expect(output).To(ContainSubstring("gregarious-scribe-24"))
				Expect(output).To(ContainSubstring("ubuntu"))
				Expect(output).To(ContainSubstring("running"))
				Expect(code).To(Equal(0))
			})

			It("writes the host to the config file", func() {
				c, err := command.LoadConfig()
				Expect(err).To(BeNil())
				Expect(c.DefaultHost).To(Equal(""))

				doCmd()

				c, err = command.LoadConfig()
				Expect(err).To(BeNil())
				Expect(c.DefaultHost).To(Equal("host-1"))
			})
		})
	})

	Context("nitro start containername", func() {
		doCmd := func() int {
			return cmd.Run([]string{"gregarious-scribe-24"})
		}

		Context("when there is no default host in the config file", func() {
			It("prints usage and exits with error", func() {
				code := doCmd()
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
				Expect(code).To(Equal(1))
			})
		})

		Context("when there is a default host in the config file", func() {
			BeforeEach(func() {
				_, err := test.WriteConfig(&command.ConfigFile{
					DefaultHost: "host-1",
				}, configPath)
				Expect(err).To(BeNil())
			})

			It("uses the default host", func() {
				statusCode = http.StatusOK
				responseBody = `{ "container": { "name": "gregarious-scribe-24", "image": "ubuntu", "state": "running" } }`

				code := doCmd()
				Expect(code).To(Equal(0))
			})
		})
	})

	Context("when not given any arguments", func() {
		It("prints usage and exits with error", func() {
			code := cmd.Run([]string{})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
			Expect(code).To(Equal(1))
		})
	})

	Context("when given > 1 argument", func() {
		It("prints usage and exits with error", func() {
			code := cmd.Run([]string{"host-1/gregarious-scribe-24", "hey", "man"})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
			Expect(code).To(Equal(1))
		})
	})
})
