package destroy_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/hosts/destroy"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hosts/destroy")
}

var _ = Describe("hosts/destroy", func() {
	var (
		cmd          *destroy.Command
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
			ghttp.VerifyRequest("DELETE", "/api/v0/hosts/foo"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &destroy.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	It("requires name as an argument", func() {
		code := cmd.Run([]string{})
		Expect(code).To(Equal(1))
		Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
	})

	Context("when host is successfully destroyed", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = `{"status":"ok"}`
		})

		It("reports success", func() {
			code := cmd.Run([]string{"foo"})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Done"))
			Expect(code).To(Equal(0))
		})
	})

	Context("when host cannot be destroyed", func() {
		BeforeEach(func() {
			statusCode = http.StatusBadRequest
			responseBody = `{"error":"invalid_request","error_description":"host not found"}`
		})

		It("displays error message", func() {
			code := cmd.Run([]string{"foo"})
			Expect(ui.ErrorWriter.String()).To(ContainSubstring("host not found"))
			Expect(code).To(Equal(1))
		})
	})
})
