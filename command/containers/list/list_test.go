package containerslist_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/containers/list"
	"github.com/nitrous-io/nitro/test"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "containerslist")
}

var _ = Describe("containerslist", func() {
	var (
		cmd        *containerslist.Command
		ui         *cli.MockUi
		srv        *ghttp.Server
		handlers   []http.HandlerFunc
		configPath string
	)

	BeforeEach(func() {
		var err error
		srv = ghttp.NewServer()
		ui = &cli.MockUi{}
		cmd = &containerslist.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
		handlers = []http.HandlerFunc{
			ghttp.VerifyHeader(http.Header{
				"Authorization": []string{"Bearer a_token"},
			}),
			ghttp.VerifyRequest("GET", "/api/v0/hosts/host-1/containers"),
		}
		configPath, err = test.WriteConfig(&command.ConfigFile{}, "")
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		srv.Close()
		os.Remove(configPath)
	})

	Context("when host is not specified", func() {
		Context("when there is no default host in the config file", func() {
			It("requires name as an argument", func() {
				code := cmd.Run([]string{})
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
				handlers = append(handlers,
					ghttp.RespondWith(http.StatusOK, `{"containers":[],"status":"ok"}`))
				srv.AppendHandlers(ghttp.CombineHandlers(handlers...))

				code := cmd.Run([]string{})
				Expect(code).To(Equal(0))
			})
		})
	})

	Context("when server responds with containers in host", func() {
		var (
			responseJSON = strings.TrimSpace(`
				{"containers":[{"id":"df5b37192a6469652b68639f5736973cf0fe9ec0fe4747ade3747d55548ccfdb","user_id":1,"host_id":25,"image":"freighter.foo","name":"foo","long_name":{"String":"foo","Valid":true},"ip_address":{"String":"10.1.0.2","Valid":true},"terminated_at":null,"touched_at":null,"created_at":"2014-07-15T05:44:09.037793Z","updated_at":"2014-07-15T05:44:09.037793Z","state":"running"}]}
				`)

			formattedOutput = strings.TrimSpace(`
Name   Image            State
foo    freighter.foo    running
`) + "\n\n"
		)

		BeforeEach(func() {
			handlers = append(handlers,
				ghttp.RespondWith(http.StatusOK, responseJSON))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
		})

		It("outputs formatted containers on success", func() {
			code := cmd.Run([]string{"host-1"})
			Expect(ui.OutputWriter.String()).To(ContainSubstring(formattedOutput))
			Expect(code).To(Equal(0))
		})

		It("writes the host to the config file", func() {
			cmd.Run([]string{"host-1"})

			configFile, err := command.LoadConfig()
			Expect(err).To(BeNil())
			Expect(configFile.DefaultHost).To(Equal("host-1"))
		})
	})

	Context("when host has no containers", func() {
		BeforeEach(func() {
			handlers = append(handlers,
				ghttp.RespondWith(http.StatusOK, `{"containers":[],"status":"ok"}`))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
		})

		It("displays no containers found", func() {
			code := cmd.Run([]string{"host-1"})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("No containers found"))
			Expect(code).To(Equal(0))
		})

		It("writes the host to the config file", func() {
			cmd.Run([]string{"host-1"})

			f, err := os.Open(configPath)
			Expect(err).To(BeNil())
			var configFile command.ConfigFile
			err = json.NewDecoder(f).Decode(&configFile)
			Expect(err).To(BeNil())
			Expect(configFile.DefaultHost).To(Equal("host-1"))
		})
	})

	Context("when server responds with an internal_server_error", func() {
		BeforeEach(func() {
			handlers = append(handlers,
				ghttp.RespondWith(http.StatusInternalServerError,
					`{"error":"internal_server_error","error_hash":"b482fd5b602650c798366fe9ee8e6cf65ef759fc"}`),
			)
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
		})

		It("displays unknown error message", func() {
			code := cmd.Run([]string{"host-1"})
			Expect(ui.ErrorWriter.String()).To(ContainSubstring("Unknown error occurred"))
			Expect(code).To(Equal(1))
		})

		It("doesn't write the host to the config file", func() {
			cmd.Run([]string{"host-1"})

			f, err := os.Open(configPath)
			Expect(err).To(BeNil())
			var configFile command.ConfigFile
			err = json.NewDecoder(f).Decode(&configFile)
			Expect(err).To(BeNil())
			Expect(configFile.DefaultHost).To(Equal(""))
		})
	})
})
