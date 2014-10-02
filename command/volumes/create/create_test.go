package create_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"

	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/volumes/create"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "volumes/create")
}

var _ = Describe("volumes/create", func() {
	var (
		cmd          *create.Command
		ui           *command.MockUI
		srv          *ghttp.Server
		statusCode   int
		responseBody string
		handlers     http.HandlerFunc
	)

	BeforeEach(func() {
		ui = &command.MockUI{&cli.MockUi{}}
		handlers = ghttp.CombineHandlers(
			func(w http.ResponseWriter, req *http.Request) {
				err := req.ParseForm()
				Expect(err).To(BeNil())
				Expect(req.PostFormValue("slug")).To(Equal("dotfiles"))
			},
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("POST", "/api/v0/hosts/myhost/volumes"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &create.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	Context("before request", func() {
		It("requires host and volume slugs", func() {
			cases := [][]string{
				{},
				{"host_only"},
				{"host", "container", "volume"},
			}
			for _, tc := range cases {
				code := cmd.Run(tc)
				Expect(code).To(Equal(1))
			}
		})
	})

	doCmd := func() int {
		return cmd.Run([]string{"myhost", "dotfiles"})
	}

	Context("successful call; volume created", func() {
		BeforeEach(func() {
			statusCode = http.StatusCreated
			responseBody = `{"status": "created","volume":{"slug":"dotfiles"}}`
		})

		It("outputs success message", func() {
			code := doCmd()
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Created new volume: 'dotfiles'"))
			Expect(code).To(Equal(0))
		})
	})

	Context("unsuccessful call", func() {
		BeforeEach(func() {
			statusCode = http.StatusBadRequest
			responseBody = `{"error":"validation_failed","error_description":"volume already exists on this host"}`
		})

		It("outputs success message", func() {
			code := doCmd()
			Expect(ui.ErrorWriter.String()).To(ContainSubstring("volume already exists"))
			Expect(code).To(Equal(1))
		})
	})
})
