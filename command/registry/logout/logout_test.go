package logout_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/registry/logout"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "registry/logout")
}

var _ = Describe("registry/logout", func() {
	var (
		cmd *logout.Command
		ui  *command.MockUI

		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string
	)

	BeforeEach(func() {
		handlers = ghttp.CombineHandlers(
			func(w http.ResponseWriter, req *http.Request) {
				// Call ParseForm() so that server.ReceivedRequests() have
				// POST params populated.
				err := req.ParseForm()
				Expect(err).To(BeNil())
			},
			ghttp.VerifyHeaderKV("Authorization", "Bearer a_token"),
			ghttp.VerifyRequest("DELETE", "/api/v0/hosts/foo-bar-123/registries"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		ui = &command.MockUI{&cli.MockUi{}}
		cmd = &logout.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	Context("when host is not specified", func() {
		doCmd := func() int {
			return cmd.Run([]string{
				"--registry=hub.example.com",
			})
		}

		It("shows the usage without making a request", func() {
			code := doCmd()
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
			Expect(code).To(Equal(1))
		})
	})

	Context("when host and registry are provided", func() {
		doCmd := func() int {
			return cmd.Run([]string{
				"foo-bar-123",
				"--registry=hub.example.com",
			})
		}

		It("populates request params", func() {
			doCmd()
			receivedRequests := srv.ReceivedRequests()
			Expect(receivedRequests).To(HaveLen(1))
			r := receivedRequests[0]
			Expect(r.FormValue("registry")).To(Equal("hub.example.com"))
		})

		Context("when request succeeds", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `{
					"status": "ok"
				}`
			})

			It("outputs success message", func() {
				code := doCmd()
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Successfully logged out from the Docker registry on foo-bar-123"))
				Expect(code).To(Equal(0))
			})
		})

		Context("when request fails", func() {
			BeforeEach(func() {
				statusCode = http.StatusBadRequest
				responseBody = `{"error":"invalid_request","error_description":"host IP unknown"}`
			})

			It("displays error message", func() {
				code := doCmd()
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("Failed to logout from Docker registry"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
