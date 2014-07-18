package remove_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/keys/remove"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "keys/remove")
}

var _ = Describe("keys/remove", func() {
	var (
		cmd      *remove.Command
		ui       *cli.MockUi
		srv      *ghttp.Server
		handlers []http.HandlerFunc
	)

	BeforeEach(func() {
		srv = ghttp.NewServer()
		ui = &cli.MockUi{}
		cmd = &remove.Command{
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

	Context("populates request correctly", func() {
		It("with no container option", func() {
			handlers = append(handlers, ghttp.VerifyRequest("DELETE", "/api/v0/keys/fprint"))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			cmd.Run([]string{"fprint"})
		})

		It("with container option", func() {
			handlers = append(handlers, ghttp.VerifyRequest("DELETE", "/api/v0/keys/fprint", "container_id=rails-test"))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			cmd.Run([]string{"fprint", "-c", "rails-test"})
		})

	})

	Context("outputs message on success", func() {
		BeforeEach(func() {
			handlers = append(handlers, ghttp.RespondWithJSONEncoded(http.StatusOK, map[string]string{"status": "ok"}))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
		})

		It("with no container option", func() {
			code := cmd.Run([]string{"fprint"})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Removed all instances of key fprint"))
			Expect(code).To(Equal(0))
		})

		It("with container option", func() {
			code := cmd.Run([]string{"-c", "node-disaster", "fprint"})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Removed key fprint from container node-disaster"))
			Expect(code).To(Equal(0))
		})
	})

	It("displays json error message", func() {
		//handlers = append(handlers, ghttp.RespondWithJSONEncoded(http.StatusBadRequest,
		//	test.ErrorJSON("validation_failed", "fingerprint not found")))
		//srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
		//code := cmd.Run([]string{"fprint", "-c", "nosuch"})
		//Expect(ui.ErrorWriter.String()).To(ContainSubstring("fingerprint not found"))
		//Expect(code).To(Equal(1))
	})
})
