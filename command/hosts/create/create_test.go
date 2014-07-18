package create_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/hosts/create"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hosts/create")
}

var _ = Describe("hosts/create", func() {
	var (
		cmd          *create.Command
		ui           *cli.MockUi
		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string
	)

	BeforeEach(func() {
		ui = &cli.MockUi{}

		handlers = ghttp.CombineHandlers(
			func(w http.ResponseWriter, req *http.Request) {
				// Call ParseForm() so that server.ReceivedRequests() have
				// POST params populated.
				err := req.ParseForm()
				Expect(err).To(BeNil())
			},
			ghttp.VerifyHeaderKV("Authorization", "Bearer a_token"),
			ghttp.VerifyRequest("POST", "/api/v0/hosts"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &create.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	It("requires size as an argument", func() {
		code := cmd.Run([]string{})
		Expect(code).To(Equal(1))
		Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
	})

	Context("hitting the server", func() {
		type createHostParams struct {
			size   string
			region string
			slug   string
		}

		doCmd := func(params *createHostParams) int {
			return cmd.Run([]string{params.size,
				"-r", params.region,
				"-s", params.slug,
			})
		}

		Context("when slug is provided", func() {
			It("populates request correctly", func() {
				doCmd(&createHostParams{size: "micro", slug: "foo", region: "apse"})
				receivedRequests := srv.ReceivedRequests()
				Expect(receivedRequests).To(HaveLen(1))
				Expect(receivedRequests[0].PostFormValue("size")).To(Equal("micro"))
				Expect(receivedRequests[0].PostFormValue("region")).To(Equal("apse"))
				Expect(receivedRequests[0].PostFormValue("slug")).To(Equal("foo"))
			})
		})

		Context("when slug is not provided", func() {
			It("populates request with a randomly generated slug", func() {
				doCmd(&createHostParams{size: "micro", region: "apse"})
				receivedRequests := srv.ReceivedRequests()
				Expect(receivedRequests).To(HaveLen(1))
				Expect(receivedRequests[0].PostFormValue("size")).To(Equal("micro"))
				Expect(receivedRequests[0].PostFormValue("region")).To(Equal("apse"))
				Expect(receivedRequests[0].PostFormValue("slug")).NotTo(Equal(""))
			})
		})

		Context("when request succeeds", func() {
			It("prints the host in the response", func() {
				statusCode = http.StatusCreated
				responseBody = `{
					"host":{"slug":"test","state":"created","size":"micro"},
					"status":"created"
				}`

				code := doCmd(&createHostParams{size: "micro", slug: "test", region: "apse"})
				Expect(ui.OutputWriter.String()).To(ContainSubstring(strings.TrimSpace(`
Slug    State      Size
test    created    micro
`)))
				Expect(code).To(Equal(0))
			})
		})

		Context("when request fails", func() {
			It("displays error message", func() {
				statusCode = http.StatusBadRequest
				responseBody = `{"error":"validation_failed","error_description":"region not found"}`

				code := doCmd(&createHostParams{size: "micro", slug: "test", region: "apse"})
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("region not found"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
