package keysclear_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/keysclear"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "keys:clear")
}

var _ = Describe("keys:clear", func() {
	var (
		cmd      *keysclear.Command
		ui       *cli.MockUi
		srv      *ghttp.Server
		handlers []http.HandlerFunc
		r        io.Reader
	)

	BeforeEach(func() {
		srv = ghttp.NewServer()
		r = strings.NewReader("YES")
		ui = &cli.MockUi{InputReader: r}
		cmd = &keysclear.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
		handlers = []http.HandlerFunc{ghttp.VerifyHeader(http.Header{
			"Authorization": []string{"Bearer a_token"},
		})}
	})

	AfterEach(func() {
		srv.Close()
	})

	Context("requires confirmation", func() {
		BeforeEach(func() {
			r = strings.NewReader("NOPE")
			ui = &cli.MockUi{InputReader: r}
			cmd.UI = ui
		})

		It("quits if confirmation is not given", func() {
			handlers = append(handlers, func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Method).To(Equal("Never Called"))
			})
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			code := cmd.Run([]string{})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("to confirm:"))
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Confirmation not given"))
			Expect(code).To(Equal(0))
		})
	})

	Context("hitting the server", func() {
		It("populates request correctly", func() {
			handlers = append(handlers, ghttp.VerifyRequest("DELETE", "/api/v0/keys"))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			code := cmd.Run([]string{})
			Expect(code).To(Equal(0))
		})

		It("outputs success msg", func() {
			handlers = append(handlers, ghttp.RespondWith(
				http.StatusOK, `{"keys":[],"status":"ok"}`))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			code := cmd.Run([]string{})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("All keys removed"))
			Expect(code).To(Equal(0))
		})

		It("displays json error message", func() {
			handlers = append(handlers, ghttp.RespondWithJSONEncoded(http.StatusBadRequest, map[string]string{"error": "validation_failed", "error_description": "container not found"}))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			code := cmd.Run([]string{})
			Expect(ui.ErrorWriter.String()).To(Equal("container not found\n"))
			Expect(code).To(Equal(1))
		})

		It("handles non-json error", func() {
			handlers = append(handlers, ghttp.RespondWith(http.StatusBadRequest, "not json"))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			code := cmd.Run([]string{})
			Expect(ui.ErrorWriter.String()).To(ContainSubstring("Unknown error"))
			Expect(code).To(Equal(1))
		})
	})
})
