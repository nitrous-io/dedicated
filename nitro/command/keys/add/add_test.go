package add_test

import (
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/dedicated/nitro/command"
	"github.com/nitrous-io/dedicated/nitro/command/keys/add"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func TestAdd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "keys/add")
}

var _ = Describe("keys/add", func() {
	var (
		cmd          *add.Command
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
				// So that server.ReceivedRequests() have params populated.
				req.ParseForm()
			},
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("PUT", "/api/v0/containers/container/keys"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)

		cmd = &add.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	Context("preconditions for request", func() {
		It("requires container and path to key arguments", func() {
			cases := [][]string{
				{},
				{"container_only"},
				{"container", "~/.ssh/a_key.pub", "excess"},
			}
			for _, tc := range cases {
				code := cmd.Run(tc)
				Expect(code).To(Equal(1))
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
			}
		})

		It("checks the presence of path to key", func() {
			code := cmd.Run([]string{"container", "/tmp/nosuchfile"})
			Expect(ui.ErrorWriter.String()).To(ContainSubstring("Could not find key"))
			Expect(code).To(Equal(1))
		})
	})

	Context("on correct arguments", func() {
		var keypath string

		makeTemp := func(content string) string {
			f, err := ioutil.TempFile("", "nitro-keysadd")
			Expect(err).To(BeNil())
			f.Write([]byte(content))
			f.Close()
			return f.Name()
		}

		BeforeEach(func() { keypath = makeTemp("somekey") })
		AfterEach(func() { os.Remove(keypath) })

		doCmd := func() int {
			return cmd.Run([]string{"container", keypath})
		}

		Context("Successful API call with key created", func() {
			BeforeEach(func() {
				statusCode = http.StatusCreated
				responseBody = `"status": "created"}`
			})

			It("outputs message on success", func() {
				code := doCmd()
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Added key"))
				Expect(code).To(Equal(0))

				reqs := srv.ReceivedRequests()
				Expect(reqs).To(HaveLen(1))
				Expect(reqs[0].PostFormValue("key")).To(Equal("somekey"))
			})
		})

		Context("Unsuccessful API call with JSON error message", func() {
			BeforeEach(func() {
				statusCode = http.StatusBadRequest
				responseBody = `{"error": "validation_failed", "error_description": "container not found"}`
			})

			It("displays json error message", func() {
				code := doCmd()
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("container not found"))
				Expect(code).To(Equal(1))
			})
		})

		Context("Unsuccessful API call with non-JSON error message", func() {
			BeforeEach(func() {
				statusCode = http.StatusInternalServerError
				responseBody = "not json"
			})

			It("handles non-json error", func() {
				code := doCmd()
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("Unknown error"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
