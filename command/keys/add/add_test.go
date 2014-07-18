package add_test

import (
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/keys/add"
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
		cmd      *add.Command
		ui       *cli.MockUi
		srv      *ghttp.Server
		handlers []http.HandlerFunc
	)

	BeforeEach(func() {
		srv = ghttp.NewServer()
		ui = &cli.MockUi{}
		cmd = &add.Command{
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

	Context("preconditions for request", func() {
		It("requires container id and path to key arguments", func() {
			cases := [][]string{
				{},
				{"container id only"},
				{"container", "~/.ssh/a_key.pub", "excess"},
			}
			for _, tc := range cases {
				code := cmd.Run(tc)
				Expect(code).To(Equal(1))
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
			}
		})

		It("checks the presence of path to key", func() {
			code := cmd.Run([]string{"cid", "/tmp/nosuchfile"})
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

		It("populates request correctly", func() {
			formValues := func(w http.ResponseWriter, r *http.Request) {
				Expect(r.FormValue("keytext")).To(Equal("somekey"))
				Expect(r.FormValue("container_id")).To(Equal("cid"))
			}
			handlers = append(handlers,
				ghttp.VerifyRequest("PUT", "/api/v0/keys"),
				formValues,
			)
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			cmd.Run([]string{"cid", keypath})
		})

		It("outputs message on success", func() {
			handlers = append(handlers, ghttp.RespondWithJSONEncoded(http.StatusCreated, map[string]string{"status": "created"}))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			code := cmd.Run([]string{"cid", keypath})
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Added key"))
			Expect(code).To(Equal(0))
		})

		It("displays json error message", func() {
			handlers = append(handlers, ghttp.RespondWithJSONEncoded(http.StatusBadRequest, map[string]string{"error": "validation_failed", "error_description": "container not found"}))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			code := cmd.Run([]string{"cid", keypath})
			Expect(ui.ErrorWriter.String()).To(ContainSubstring("container not found"))
			Expect(code).To(Equal(1))
		})

		It("handles non-json error", func() {
			handlers = append(handlers, ghttp.RespondWith(http.StatusBadRequest, "not json"))
			srv.AppendHandlers(ghttp.CombineHandlers(handlers...))
			code := cmd.Run([]string{"cid", keypath})
			Expect(ui.ErrorWriter.String()).To(ContainSubstring("Unknown error"))
			Expect(code).To(Equal(1))
		})
	})
})
