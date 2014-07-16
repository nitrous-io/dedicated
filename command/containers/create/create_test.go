package containerscreate_test

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/containers/create"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "containerscreate")
}

var _ = Describe("containerscreate", func() {
	var (
		cmd        *containerscreate.Command
		ui         *cli.MockUi
		srv        *ghttp.Server
		handlers   []http.HandlerFunc
		configPath string
	)

	writeConfig := func(conf *command.ConfigFile, path string) string {
		var f *os.File
		var err error
		if path == "" {
			f, err = ioutil.TempFile("", "nitrocfg")
		} else {
			f, err = os.Create(path)
		}
		Expect(err).To(BeNil())
		defer f.Close()
		os.Setenv("NITRO_CONFIG_PATH", f.Name())
		err = json.NewEncoder(f).Encode(conf)
		Expect(err).To(BeNil())
		command.Env = command.ConfigureEnvironment()
		return f.Name()
	}

	BeforeEach(func() {
		srv = ghttp.NewServer()
		ui = &cli.MockUi{}
		cmd = &containerscreate.Command{
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
		configPath = writeConfig(&command.ConfigFile{}, "")
	})

	AfterEach(func() {
		os.Remove(configPath)
	})

	Context("when not all options are specified", func() {
		It("populates request with only given params", func() {
			handlers = append(handlers, func(w http.ResponseWriter, r *http.Request) {
				Expect(r.FormValue("name")).To(Equal("foo"))
			})
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			cmd.Run([]string{
				"--host=host-1",
				"--name=foo",
			})
		})
	})

	Context("when name is not specified", func() {
		It("populates request with a randomly generated name", func() {
			handlers = append(handlers, func(w http.ResponseWriter, r *http.Request) {
				Expect(r.FormValue("name")).NotTo(Equal(""))
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
				"--name=foo",
				"--longname=foobar",
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
				writeConfig(&command.ConfigFile{
					DefaultHost: "host-1",
				}, configPath)
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
				"--name=foo",
				"--longname=foobar",
				"--image=ubuntu",
			})
		}

		It("populates request with all params", func() {
			handlers = append(handlers, func(w http.ResponseWriter, r *http.Request) {
				Expect(r.FormValue("name")).To(Equal("foo"))
				Expect(r.FormValue("longname")).To(Equal("foobar"))
				Expect(r.FormValue("image")).To(Equal("ubuntu"))
			})
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			doCmd()
		})

		Context("when request succeeds", func() {
			BeforeEach(func() {
				handlers = append(handlers,
					ghttp.RespondWith(http.StatusCreated, `{"status":"created"}`))
				srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			})

			It("reports success", func() {
				code := doCmd()
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Done"))
				Expect(code).To(Equal(0))
			})

			It("writes the host to the config file", func() {
				doCmd()

				f, err := os.Open(configPath)
				Expect(err).To(BeNil())
				var configFile command.ConfigFile
				err = json.NewDecoder(f).Decode(&configFile)
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
