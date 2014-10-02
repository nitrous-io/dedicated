package create_test

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
	"github.com/nitrous-io/nitro/command/hosts/create"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hosts/create")
}

func getCmd(output io.Writer, server string) *create.Command {
	ui := cli.BasicUi{Writer: output}
	cmd := &create.Command{
		UI:     &command.BasicUI{ui},
		Server: server,
		Client: httpclient.New("a_token"),
	}
	return cmd
}

var _ = Describe("validation", func() {
	It("requires size as an argument", func() {
		output := bytes.NewBuffer(nil)
		cmd := getCmd(output, "")

		code := cmd.Run([]string{})
		Expect(code).To(Equal(1))
		Expect(output.String()).To(ContainSubstring("Usage:"))
	})

	It("requires region as an argument", func() {
		output := bytes.NewBuffer(nil)
		cmd := getCmd(output, "")

		code := cmd.Run([]string{"512mb"})
		Expect(code).To(Equal(1))
		Expect(output.String()).To(ContainSubstring("Usage:"))
	})
})

var _ = Describe("hosts/create", func() {
	var (
		srv          *ghttp.Server
		statusCode   int
		responseBody string
	)

	BeforeEach(func() {
		handlers := ghttp.CombineHandlers(
			func(w http.ResponseWriter, req *http.Request) {
				// Call ParseForm() so that server.ReceivedRequests() have
				// POST params populated.
				err := req.ParseForm()
				Expect(err).To(BeNil())
			},
			ghttp.VerifyHeaderKV("Authorization", "Bearer a_token"),
			ghttp.VerifyRequest("POST", "/api/v0/hosts"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv = ghttp.NewServer()
		srv.AppendHandlers(handlers)
	})

	AfterEach(func() {
		srv.Close()
	})

	It("populates request with provided slug", func() {
		output := bytes.NewBuffer(nil)
		cmd := getCmd(output, srv.URL())

		cmd.Run([]string{"micro", "apse", "-s", "foo"})

		receivedRequests := srv.ReceivedRequests()
		Expect(receivedRequests).To(HaveLen(1))
		Expect(receivedRequests[0].PostFormValue("size")).To(Equal("micro"))
		Expect(receivedRequests[0].PostFormValue("region")).To(Equal("apse"))
		Expect(receivedRequests[0].PostFormValue("slug")).To(Equal("foo"))
	})

	It("populates request with a randomly generated slug when slug is not provided", func() {
		output := bytes.NewBuffer(nil)
		cmd := getCmd(output, srv.URL())

		cmd.Run([]string{"micro", "apse"})

		receivedRequests := srv.ReceivedRequests()
		Expect(receivedRequests).To(HaveLen(1))
		Expect(receivedRequests[0].PostFormValue("size")).To(Equal("micro"))
		Expect(receivedRequests[0].PostFormValue("region")).To(Equal("apse"))
		Expect(receivedRequests[0].PostFormValue("slug")).NotTo(Equal(""))
	})

	It("prints out stream output and host info", func() {
		statusCode = http.StatusOK
		responseBody = `
		{"stream":"starting: create droplet"}
		{"stream":"finished: create droplet"}
		{"host":{"slug":"test","state":"created","size":"micro"}, "status":"created"}
		`

		output := bytes.NewBuffer(nil)
		cmd := getCmd(output, srv.URL())

		code := cmd.Run([]string{"micro", "apse", "-s", "test"})
		Expect(code).To(Equal(0), "unexpected exit code")
		Expect(output.String()).To(ContainSubstring(`starting: create droplet`))
		Expect(output.String()).To(ContainSubstring(strings.TrimSpace(`
Slug    State      Size
test    created    micro
`)))
	})

	It("displays error message when the server receives a bad request", func() {
		statusCode = http.StatusBadRequest
		responseBody = `{"error":"validation_failed","error_description":"region not found"}`

		output := bytes.NewBuffer(nil)
		cmd := getCmd(output, srv.URL())

		code := cmd.Run([]string{"micro", "apse", "-s", "test"})
		Expect(code).To(Equal(1), "unexpected exit code")
		Expect(output.String()).To(ContainSubstring("region not found"), "missing error message")
	})

	It("displays error message when the build failed", func() {
		statusCode = http.StatusOK
		responseBody = `
		{"error":"hostbuilder: failed to build because reasons"}
		{"error":"internal_server_error","error_hash":"obscure error"}
		`

		output := bytes.NewBuffer(nil)
		cmd := getCmd(output, srv.URL())

		code := cmd.Run([]string{"micro", "apse", "-s", "test"})
		Expect(code).To(Equal(1), "unexpected exit code")
		Expect(output.String()).To(ContainSubstring("failed to build because reasons"), "missing error message")
		Expect(output.String()).To(ContainSubstring("obscure error"), "missing error message")
	})
})
