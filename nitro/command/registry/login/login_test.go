package login_test

import (
	"io"
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command/registry/login"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "registry/login")
}

var _ = Describe("registry/login", func() {
	var (
		cmd *login.Command
		ui  *cli.MockUi
		r   *io.PipeReader
		w   *io.PipeWriter

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
			ghttp.VerifyRequest("POST", "/api/v0/hosts/foo-bar-123/registries"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		r, w = io.Pipe()
		ui = &cli.MockUi{InputReader: r}
		cmd = &login.Command{
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
				"--username=foo",
				"--password=123456",
				"--email=foo@example.com",
				"--registry=hub.example.com",
			})
		}

		It("shows the usage without making a request", func() {
			code := doCmd()
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
			Expect(code).To(Equal(1))
		})
	})

	Context("when all options are provided", func() {
		doCmd := func() int {
			return cmd.Run([]string{
				"foo-bar-123",
				"--username=foo",
				"--password=123456",
				"--email=foo@example.com",
				"--registry=hub.example.com",
			})
		}

		It("populates request params", func() {
			doCmd()
			receivedRequests := srv.ReceivedRequests()
			Expect(receivedRequests).To(HaveLen(1))
			r := receivedRequests[0]
			Expect(r.PostFormValue("username")).To(Equal("foo"))
			Expect(r.PostFormValue("password")).To(Equal("123456"))
			Expect(r.PostFormValue("email")).To(Equal("foo@example.com"))
			Expect(r.PostFormValue("url")).To(Equal("hub.example.com"))
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
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Successfully logged in to Docker registry on foo-bar-123"))
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
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("Failed to login to Docker registry"))
				Expect(code).To(Equal(1))
			})
		})
	})

	Context("when no registry URL is provided", func() {
		doCmd := func() int {
			return cmd.Run([]string{
				"foo-bar-123",
				"--username=foo",
				"--password=123456",
				"--email=foo@example.com",
			})
		}

		It("populates url param with default docker registry", func() {
			go w.Write([]byte("\n"))
			doCmd()
			receivedRequests := srv.ReceivedRequests()
			Expect(receivedRequests).To(HaveLen(1))
			r := receivedRequests[0]
			Expect(r.PostFormValue("url")).To(Equal("https://index.docker.io/v1/"))
		})
	})
})
