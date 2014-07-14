package hostscreate_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/hostscreate"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hostscreate")
}

var _ = Describe("hosts:create", func() {
	var (
		cmd      *hostscreate.Command
		ui       *cli.MockUi
		srv      *ghttp.Server
		handlers []http.HandlerFunc
	)

	BeforeEach(func() {
		srv = ghttp.NewServer()
		ui = &cli.MockUi{}
		cmd = &hostscreate.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
		handlers = []http.HandlerFunc{
			ghttp.VerifyHeader(http.Header{
				"Authorization": []string{"Bearer a_token"},
			}),
			ghttp.VerifyRequest("POST", "/api/v0/hosts"),
		}
	})

	It("requires size as an argument", func() {
		code := cmd.Run([]string{})
		Expect(code).To(Equal(1))
		Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
	})

	Context("hitting the server", func() {
		doCmd := func() int {
			return cmd.Run([]string{"micro",
				"-r", "apse",
				"-n", "foo",
				"-L", "foobar",
			})
		}

		It("populates request correctly", func() {
			handlers = append(handlers, func(w http.ResponseWriter, r *http.Request) {
				Expect(r.FormValue("size")).To(Equal("micro"))
				Expect(r.FormValue("region")).To(Equal("apse"))
				Expect(r.FormValue("name")).To(Equal("foo"))
				Expect(r.FormValue("longname")).To(Equal("foobar"))
			})
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			doCmd()
		})

		Context("when request succeeds", func() {
			It("reports success", func() {
				handlers = append(handlers,
					ghttp.RespondWith(http.StatusCreated, `{"status":"created"}`))
				srv.AppendHandlers(ghttp.CombineHandlers(handlers...))

				code := doCmd()
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Done"))
				Expect(code).To(Equal(0))
			})
		})

		Context("when request fails", func() {
			It("displays error message", func() {
				handlers = append(handlers,
					ghttp.RespondWith(http.StatusBadRequest,
						`{"error":"validation_failed","error_description":"region not found"}`),
				)
				srv.AppendHandlers(ghttp.CombineHandlers(handlers...))

				code := doCmd()
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("region not found"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
