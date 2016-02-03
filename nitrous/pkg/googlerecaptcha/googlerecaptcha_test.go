package googlerecaptcha_test

import (
	"net/http"
	"testing"

	"github.com/nitrous-io/dedicated/nitrous/pkg/googlerecaptcha"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func TestContext(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "googlerecaptcha")
}

var _ = Describe("googlerecaptcha", func() {
	var (
		verifier  *googlerecaptcha.GoogleReCaptchaVerifier
		server    *ghttp.Server
		parseForm http.HandlerFunc
		secret    string
		token     string
	)

	BeforeEach(func() {
		secret = "secret"
		token = "token"
		verifier = googlerecaptcha.NewVerifier("secret")

		server = ghttp.NewServer()
		verifier.SetURL(server.URL())
		parseForm = func(w http.ResponseWriter, r *http.Request) {
			r.ParseForm()
		}
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("Verify", func() {
		It("returns an error when attempting to verify an empty token", func() {
			err := verifier.Verify("")
			Expect(err).To(Equal(googlerecaptcha.ErrTokenRequired))
		})

		It("requests with correct params", func() {
			server.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/"),
				parseForm,
				ghttp.RespondWith(http.StatusOK, `{"success": true}`),
			))

			verifier.Verify(token)

			Expect(server.ReceivedRequests()).Should(HaveLen(1))
			req := server.ReceivedRequests()[0]
			Expect(req.Form.Get("secret")).To(Equal(secret))
			Expect(req.Form.Get("response")).To(Equal(token))
		})

		It("does not return error when it succeeds", func() {
			server.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/"),
				parseForm,
				ghttp.RespondWith(http.StatusOK, `{"success": true}`),
			))

			Expect(verifier.Verify(token)).NotTo(HaveOccurred())
		})

		It("returns an error when it fails with empty body", func() {
			server.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/"),
				parseForm,
				ghttp.RespondWith(http.StatusOK, `{}`),
			))

			Expect(verifier.Verify(token)).To(HaveOccurred())
		})

		It("returns an error when it fails", func() {
			server.AppendHandlers(ghttp.CombineHandlers(
				ghttp.VerifyRequest("POST", "/"),
				parseForm,
				ghttp.RespondWith(http.StatusOK, `{"success": false, "error-codes": ["missing-input-secret"]}`),
			))

			Expect(verifier.Verify(token)).To(HaveOccurred())
		})
	})
})
