package list_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command/registry/list"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "registry/list")
}

var _ = Describe("registry/list", func() {
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
			ghttp.VerifyRequest("GET", "/api/v0/hosts/foo/registries"),
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
				responseBody = `{"registries":[
					{"url":"hub.example.com","username":"test","email":"test@example.com"},
					{"url":"hub1.example.com","username":"test1","email":"test1@example.com"}
				]}`
			})

			It("outputs formatted registries on success", func() {
				code := cmd.Run([]string{"foo"})
				Expect(ui.OutputWriter.String()).To(ContainSubstring(strings.TrimSpace(`
Registry            Username   Email
hub.example.com     test       test@example.com
hub1.example.com    test1      test1@example.com
`)))
				Expect(code).To(Equal(0))
			})
		})

		Context("when user has no registries", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `{"registries":[]}`
			})

			It("shows not logged in to any registries", func() {
				code := cmd.Run([]string{"foo"})
				Expect(ui.OutputWriter.String()).To(ContainSubstring("You are not logged in to any registries"))
				Expect(code).To(Equal(0))
			})
		})

		Context("when server responds with an internal_server_error", func() {
			BeforeEach(func() {
				statusCode = http.StatusInternalServerError
				responseBody = `{"error":"internal_server_error","error_hash":"b482fd5b602650c798366fe9ee8e6cf65ef759fc"}`
			})

			It("displays an error message containing an error code", func() {
				code := cmd.Run([]string{"foo"})
				Expect(ui.ErrorWriter.String()).To(MatchRegexp(`error code: \w+`))
				Expect(code).To(Equal(1))
			})
		})
	})
})
