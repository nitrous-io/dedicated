package list_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/volumes/list"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "volumes/list")
}

var _ = Describe("volumes/list", func() {
	var (
		cmd          *list.Command
		ui           *command.MockUI
		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string

		responseJSON   = `{"status":"ok","volumes":[{"id":1,"name":"dotfiles","slug":"dotfiles","created_at":"2014-07-23T19:38:19.421095Z","updated_at":"2014-07-23T19:38:19.421095Z"},{"id":7,"name":"music","slug":"music","created_at":"2014-07-24T17:31:35.615001Z","updated_at":"2014-07-24T17:31:35.615001Z"}]}`
		responseOutput = "Slug\ndotfiles\nmusic\n"
	)

	BeforeEach(func() {
		ui = &command.MockUI{&cli.MockUi{}}
		handlers = ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("GET", "/api/v0/hosts/myhost/volumes"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &list.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}
	})

	AfterEach(func() {
		statusCode = http.StatusOK
		responseBody = `{"volumes":[],"status":"ok"}`

		srv.Close()
	})

	doCmd := func() int {
		return cmd.Run([]string{"myhost"})
	}

	Context("successful API call which returns volumes", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = responseJSON
		})

		It("outputs formatted volumes on success", func() {
			code := doCmd()
			Expect(code).To(Equal(0))
			Expect(ui.OutputWriter.String()).To(ContainSubstring(responseOutput))
		})
	})

	Context("Successful API call which returns no volumes", func() {
		BeforeEach(func() {
			statusCode = http.StatusOK
			responseBody = `{"volumes":[],"status":"ok"}`
		})

		It("displays no volumes found", func() {
			code := doCmd()
			Expect(ui.OutputWriter.String()).To(ContainSubstring("No volumes found\n"))
			Expect(code).To(Equal(0))
		})
	})

	Context("Unsuccessful API call which returns a JSON error message", func() {
		BeforeEach(func() {
			statusCode = http.StatusBadRequest
			responseBody = `{"error": "validation_failed", "error_description": "host not found"}`
		})

		It("displays json error message", func() {
			code := doCmd()
			Expect(ui.ErrorWriter.String()).To(Equal("host not found\n"))
			Expect(code).To(Equal(1))
		})
	})
})
