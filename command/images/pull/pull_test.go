package pull_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/images/pull"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "images/pull")
}

var _ = Describe("images/pull", func() {
	var (
		cmd          *pull.Command
		ui           *command.MockUI
		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string
	)

	BeforeEach(func() {
		ui = &command.MockUI{&cli.MockUi{}}
		handlers = ghttp.CombineHandlers(
			func(w http.ResponseWriter, req *http.Request) {
				Expect(req.FormValue("repository")).To(Equal("scratch"))
			},
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("POST", "/api/v0/hosts/myhost/images/pull"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &pull.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	Context("nitro images:pull host image", func() {
		doCmd := func() int {
			return cmd.Run([]string{"myhost", "scratch"})
		}

		Context("successful pull", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `{"image":{"id":"123456snip","name":"scratch","tag":"latest","created_at":1371157430,"actual_size":123,"virtual_size":456},"status":"OK"}`
			})

			It("outputs image details", func() {
				code := doCmd()
				Expect(ui.OutputWriter.String()).To(ContainSubstring(`
ID            Name       Tag       Created       Size   Virtual Size
123456snip    scratch    latest    1371157430    123    456
`))
				Expect(code).To(Equal(0))
			})
		})

		Context("JSON response with error", func() {
			BeforeEach(func() {
				statusCode = http.StatusBadRequest
				responseBody = `{"error": "validation_failed", "error_description": "image not found"}`
			})

			It("outputs error message", func() {
				code := doCmd()
				Expect(ui.ErrorWriter.String()).To(Equal("image not found\n"))
				Expect(code).To(Equal(1))
			})
		})
	})

	Context("when given wrong arguments", func() {
		It("prints usage and exits with error", func() {
			code := cmd.Run([]string{})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
			Expect(code).To(Equal(1))
		})
	})

	Context("Help", func() {
		It("outputs usage", func() {
			Expect(cmd.Help()).To(ContainSubstring("Usage: nitro images:pull"))
		})
	})
})
