package regions_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/regions"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "regions")
}

var _ = Describe("regions", func() {
	var (
		cmd          *regions.Command
		ui           *cli.MockUi
		srv          *ghttp.Server
		statusCode   int
		responseBody string
	)

	BeforeEach(func() {
		ui = &cli.MockUi{}

		handlers := ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/api/v0/regions"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &regions.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	Context("when server responds with regions", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = `{"regions":[
					{"id":1,"slug":"apse","name":"Southeast Asia","rank":1},
					{"id":2,"slug":"nyc","name":"US East (New York)","rank":2}
				]}`
		})

		It("outputs formatted regions on success", func() {
			code := cmd.Run([]string{})
			Expect(ui.OutputWriter.String()).To(ContainSubstring(strings.TrimSpace(`
Slug    Name
apse    Southeast Asia
nyc     US East (New York)
`)))
			Expect(code).To(Equal(0))
		})
	})

	Context("when server responds with empty regions object", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = `{"regions":[]}`
		})

		It("displays no regions found", func() {
			code := cmd.Run([]string{})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("No regions found"))
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
