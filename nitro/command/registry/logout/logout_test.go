package logout_test

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command"
	"github.com/nitrous-io/dedicated/nitro/command/registry/logout"
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
		r   *io.PipeReader
		w   *io.PipeWriter

		srv          *ghttp.Server
		statusCode   int
		responseBody string
	)

	BeforeEach(func() {
		handlers := ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer a_token"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		r, w = io.Pipe()
		ui = &command.MockUI{&cli.MockUi{InputReader: r}}

		cmd = &logout.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
	})

	AfterEach(func() {
		r.Close()
		w.Close()
		srv.Close()
	})

	doCmd := func(host, registry string) int {
		args := []string{}
		if host != "" {
			args = append(args, host)
		}
		if registry != "" {
			args = append(args, fmt.Sprintf("--registry=%s", registry))
		}
		return cmd.Run(args)
	}

	Context("when host is not specified", func() {
		It("shows the usage without making a request", func() {
			code := doCmd("", "hub.example.com")
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
			Expect(code).To(Equal(1))
		})
	})

	Context("when host and registry are provided", func() {
		Context("when request succeeds", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `{
					"status": "ok"
				}`
			})

			It("makes a request with given registry URL escaped", func() {
				doCmd("foo-bar-123", "https://hub.example.com")
				receivedRequests := srv.ReceivedRequests()
				Expect(receivedRequests).To(HaveLen(1))
				r := receivedRequests[0]
				Expect(r.Method).To(Equal("DELETE"))
				Expect(r.URL.Path).To(Equal("/api/v0/hosts/foo-bar-123/registries/https%3A%2F%2Fhub.example.com"))
			})

			It("outputs success message", func() {
				code := doCmd("foo-bar-123", "http://hub.example.com")
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Successfully logged out from the Docker registry on foo-bar-123"))
				Expect(code).To(Equal(0))
			})
		})

		Context("when request fails", func() {
			BeforeEach(func() {
				statusCode = http.StatusBadRequest
				responseBody = `{"error":"invalid_request","error_description":"host IP unknown"}`
			})

			It("makes a request with given registry URL", func() {
				doCmd("foo-bar-123", "hub.example.com")
				receivedRequests := srv.ReceivedRequests()
				Expect(receivedRequests).To(HaveLen(1))
				r := receivedRequests[0]
				Expect(r.Method).To(Equal("DELETE"))
				Expect(r.URL.Path).To(Equal("/api/v0/hosts/foo-bar-123/registries/hub.example.com"))
			})

			It("displays error message", func() {
				code := doCmd("foo-bar-123", "hub.example.com")
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("Failed to logout from Docker registry"))
				Expect(code).To(Equal(1))
			})
		})
	})

	Context("when registry is not provided", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = `{
					"status": "ok"
				}`
		})

		It("asks users confirmation to use Docker Hub registry", func() {
			go w.Write([]byte("n\n"))
			code := doCmd("foo-bar-123", "")
			Expect(ui.ErrorWriter.String()).To(ContainSubstring("Registry logout canceled"))
			Expect(code).To(Equal(1))

			go w.Write([]byte("y\n"))
			code = doCmd("foo-bar-123", "")
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Successfully logged out from the Docker registry on foo-bar-123"))
			Expect(code).To(Equal(0))

			// makes a request with Docker Hub as the registry URL
			receivedRequests := srv.ReceivedRequests()
			Expect(receivedRequests).To(HaveLen(1))
			r := receivedRequests[0]
			Expect(r.Method).To(Equal("DELETE"))
			Expect(r.URL.Path).To(Equal("/api/v0/hosts/foo-bar-123/registries/https%3A%2F%2Findex.docker.io%2Fv1%2F"))
		})
	})
})
