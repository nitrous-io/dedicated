package plans_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/plans"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "plans")
}

var _ = Describe("plans", func() {
	var (
		cmd          *plans.Command
		ui           *cli.MockUi
		srv          *ghttp.Server
		statusCode   int
		responseBody string
	)

	BeforeEach(func() {
		ui = &cli.MockUi{}

		handlers := ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/api/v0/plans"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &plans.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	Context("when server responds with plans", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = `{"plans":[
					{"id":1,"name":"nano"},
					{"id":2,"name":"micro"},
					{"id":3,"name":"small"},
					{"id":4,"name":"medium"},
					{"id":5,"name":"large"}
				]}`
		})

		It("outputs formatted plans on success", func() {
			code := cmd.Run([]string{})
			Expect(ui.OutputWriter.String()).To(ContainSubstring(strings.TrimSpace(`
Name
nano
micro
small
medium
large
`)))
			Expect(code).To(Equal(0))
		})
	})

	Context("when server responds with empty plans object", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = `{"plans":[]}`
		})

		It("displays no plans found", func() {
			code := cmd.Run([]string{})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("No plans found"))
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
