package commit_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/containers/commit"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "containers/commit")
}

var _ = Describe("containers/commit", func() {
	var (
		cmd          *commit.Command
		ui           *command.MockUI
		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string
		expectedName string
	)

	BeforeEach(func() {
		ui = &command.MockUI{&cli.MockUi{}}
		handlers = ghttp.CombineHandlers(
			func(w http.ResponseWriter, req *http.Request) {
				if expectedName == "" {
					Expect(req.FormValue("name")).NotTo(BeEmpty())
				} else {
					Expect(req.FormValue("name")).To(Equal(expectedName))
				}
			},
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("POST", "/api/v0/containers/scratchcon/commit"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &commit.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	Context("nitro commit container", func() {
		Context("successful commit with name specified", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				expectedName = "scrimage"
				responseBody = `{"image":{"id":"123456snip","name":"scrimage","tag":"latest","created_at":1371157430,"actual_size":123,"virtual_size":456},"status":"OK"}`
			})

			It("outputs image details", func() {
				code := cmd.Run([]string{"scratchcon", "-t", "scrimage"})
				Expect(ui.OutputWriter.String()).To(ContainSubstring(`
ID            Name        Tag       Created       Size   Virtual Size
123456snip    scrimage    latest    1371157430    123    456
`))
				Expect(code).To(Equal(0))
			})
		})

		Context("successful commit with no name specified", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				expectedName = ""
				responseBody = `{"image":{"id":"123456snip","name":"random","tag":"latest","created_at":1371157430,"actual_size":123,"virtual_size":456},"status":"OK"}`
			})

			It("outputs image details", func() {
				code := cmd.Run([]string{"scratchcon"})
				Expect(ui.OutputWriter.String()).To(ContainSubstring(`
ID            Name      Tag       Created       Size   Virtual Size
123456snip    random    latest    1371157430    123    456
`))
				Expect(code).To(Equal(0))
			})
		})

		Context("JSON response with error", func() {
			BeforeEach(func() {
				expectedName = ""
				statusCode = http.StatusBadRequest
				responseBody = `{"error": "invalid_request", "error_description": "image already exists"}`
			})

			It("outputs error message", func() {
				code := cmd.Run([]string{"scratchcon"})
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("image already exists\n"))
				Expect(code).To(Equal(1))
			})
		})
	})

	Context("when given wrong arguments", func() {
		It("prints usage and exits with error", func() {
			for _, t := range [][]string{{}, {"container", "extra"}} {
				code := cmd.Run(t)
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
				Expect(code).To(Equal(1))
			}
		})
	})

	Context("Help", func() {
		It("outputs usage", func() {
			Expect(cmd.Help()).To(ContainSubstring("Usage: nitro commit"))
		})
	})
})
