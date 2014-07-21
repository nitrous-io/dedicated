package list_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/hosts/list"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hosts/list")
}

var _ = Describe("hosts/list", func() {
	var (
		cmd          *list.Command
		ui           *cli.MockUi
		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string
	)

	BeforeEach(func() {
		ui = &cli.MockUi{}

		handlers = ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer a_token"),
			ghttp.VerifyRequest("GET", "/api/v0/hosts"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &list.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	Context("hitting the server", func() {
		Context("when server responds with hosts", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `{"hosts":[
					{"slug":"host-1","state":"created","size":"nano"},
					{"slug":"host-2","state":"born","size":"micro"}
				]}`
			})

			It("outputs formatted hosts on success", func() {
				code := cmd.Run([]string{})
				Expect(ui.OutputWriter.String()).To(ContainSubstring(strings.TrimSpace(`
Slug      State      Size
host-1    created    nano
host-2    born       micro
`)))
				Expect(code).To(Equal(0))
			})
		})

		Context("when user has no hosts", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `{"hosts":[]}`
			})

			It("displays no hosts found", func() {
				code := cmd.Run([]string{})
				Expect(ui.OutputWriter.String()).To(ContainSubstring("No hosts found"))
				Expect(code).To(Equal(0))
			})
		})

		Context("when server responds with an internal_server_error", func() {
			BeforeEach(func() {
				statusCode = http.StatusInternalServerError
				responseBody = `{"error":"internal_server_error","error_hash":"b482fd5b602650c798366fe9ee8e6cf65ef759fc"}`
			})

			It("displays unknown error message", func() {
				code := cmd.Run([]string{})
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("Unknown error occurred"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
