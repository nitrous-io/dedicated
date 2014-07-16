package create_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/containers/create"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "create")
}

var _ = Describe("create", func() {
	var (
		cmd      *create.Command
		ui       *cli.MockUi
		srv      *ghttp.Server
		handlers []http.HandlerFunc
	)

	BeforeEach(func() {
		srv = ghttp.NewServer()
		ui = &cli.MockUi{}
		cmd = &create.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
		handlers = []http.HandlerFunc{
			ghttp.VerifyHeader(http.Header{
				"Authorization": []string{"Bearer a_token"},
			}),
			ghttp.VerifyRequest("POST", "/api/v0/hosts/host-1/containers"),
		}
	})

	Context("when not all options are specified", func() {
		It("populates request with only given params", func() {
			handlers = append(handlers, func(w http.ResponseWriter, r *http.Request) {
				Expect(r.FormValue("name")).To(Equal("foo"))
			})
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			cmd.Run([]string{
				"--host=host-1",
				"--name=foo",
			})
		})
	})

	Context("when host is not specified", func() {
		It("returns an error message without making a request", func() {
			code := cmd.Run([]string{
				"--name=foo",
				"--longname=foobar",
				"--image=ubuntu",
			})
			Expect(ui.ErrorWriter.String()).To(ContainSubstring("host required"))
			Expect(code).To(Equal(1))
		})
	})

	Context("when all options are provided", func() {
		doCmd := func() int {
			return cmd.Run([]string{
				"--host=host-1",
				"--name=foo",
				"--longname=foobar",
				"--image=ubuntu",
			})
		}

		It("populates request with all params", func() {
			handlers = append(handlers, func(w http.ResponseWriter, r *http.Request) {
				Expect(r.FormValue("name")).To(Equal("foo"))
				Expect(r.FormValue("longname")).To(Equal("foobar"))
				Expect(r.FormValue("image")).To(Equal("ubuntu"))
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
						`{"error":"validation_failed","error_description":"image required"}`),
				)
				srv.AppendHandlers(ghttp.CombineHandlers(handlers...))

				code := doCmd()
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("image required"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
