package create_test

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/containers/create"
	"github.com/nitrous-io/nitro/test"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "containers/create")
}

var _ = Describe("containers/create", func() {
	var (
		cmd        *create.Command
		ui         *cli.MockUi
		srv        *ghttp.Server
		handlers   []http.HandlerFunc
		configPath string
	)

	BeforeEach(func() {
		var err error
		srv = ghttp.NewServer()
		ui = &cli.MockUi{}
		cmd = &create.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
		handlers = []http.HandlerFunc{
			ghttp.VerifyHeader(http.Header{
				"Authorization": []string{"Bearer a_token"},
			}),
			ghttp.VerifyRequest("POST", "/api/v0/hosts/host-1/containers"),
		}
		configPath, err = test.WriteConfig(&command.ConfigFile{}, "")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		os.Remove(configPath)
	})

	Context("when not all options are specified", func() {
		It("populates request with only given params", func() {
			handlers = append(handlers, func(w http.ResponseWriter, r *http.Request) {
				Expect(r.FormValue("slug")).To(Equal("foo"))
			})
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			cmd.Run([]string{
				"--host=host-1",
				"--slug=foo",
			})
		})
	})

	Context("when slug is not specified", func() {
		It("populates request with a randomly generated slug", func() {
			handlers = append(handlers, func(w http.ResponseWriter, r *http.Request) {
				Expect(r.FormValue("slug")).NotTo(Equal(""))
			})
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			cmd.Run([]string{
				"--host=host-1",
			})
		})
	})

	Context("when host is not specified", func() {
		doCmd := func() int {
			return cmd.Run([]string{
				"--slug=foo",
				"--image=ubuntu",
			})
		}
		Context("when there is no default host in the config file", func() {
			It("returns an error message without making a request", func() {
				code := doCmd()
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("Please specify a host"))
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
				handlers = append(handlers,
					ghttp.RespondWith(http.StatusCreated, `{"status":"created"}`))
				srv.AppendHandlers(ghttp.CombineHandlers(handlers...))

				code := doCmd()
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Done"))
				Expect(code).To(Equal(0))
			})
		})
	})

	Context("when all options are provided", func() {
		doCmd := func() int {
			return cmd.Run([]string{
				"--host=host-1",
				"--slug=foo",
				"--image=ubuntu",
			})
		}

		It("populates request with all params", func() {
			handlers = append(handlers, func(w http.ResponseWriter, r *http.Request) {
				Expect(r.FormValue("slug")).To(Equal("foo"))
				Expect(r.FormValue("image")).To(Equal("ubuntu"))
			})
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			doCmd()
		})

		Context("when request succeeds", func() {
			BeforeEach(func() {
				response := `{
					"status": "created",
					"container": { "slug": "wild-rocket-23", "image": "ubuntu", "state": "running" }
				}`
				handlers = append(handlers, ghttp.RespondWith(http.StatusCreated, response))
				srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			})

			It("reports the container's slug", func() {
				code := doCmd()
				Expect(ui.OutputWriter.String()).To(ContainSubstring("wild-rocket-23"))
				Expect(code).To(Equal(0))
			})

			It("writes the host to the config file", func() {
				doCmd()

				configFile, err := command.LoadConfig()
				Expect(err).To(BeNil())
				Expect(configFile.DefaultHost).To(Equal("host-1"))
			})
		})

		Context("when request fails", func() {
			BeforeEach(func() {
				handlers = append(handlers,
					ghttp.RespondWith(http.StatusBadRequest,
						`{"error":"validation_failed","error_description":"image required"}`),
				)
				srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			})

			It("displays error message", func() {
				code := doCmd()
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("image required"))
				Expect(code).To(Equal(1))
			})

			It("doesn't write the host to the config file", func() {
				doCmd()

				f, err := os.Open(configPath)
				Expect(err).To(BeNil())
				var configFile command.ConfigFile
				err = json.NewDecoder(f).Decode(&configFile)
				Expect(err).To(BeNil())
				Expect(configFile.DefaultHost).To(Equal(""))
			})
		})
	})
})
