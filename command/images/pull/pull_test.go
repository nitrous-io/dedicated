package pull_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/images/pull"
	"github.com/nitrous-io/nitrous-common/httpclient"
	"github.com/onsi/gomega/ghttp"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "images/pull")
}

var _ = Describe("images/pull", func() {
	It("outputs formatted download progress", func() {
		pullResponse := `
		{"status":"Pulling..."}
		{"status":"Pulling", "progress":"1 B/ 100 B", "progressDetail":{"current":1, "total":100}}
		`
		nitrousSrv := setupNitrous("myhost", http.StatusOK, pullResponse)
		defer nitrousSrv.Close()

		// create a tmp file to store pull output
		out, err := ioutil.TempFile("/tmp", "images-pull")
		outName := out.Name()
		Expect(err).To(BeNil(), "failed to create tmp file for storing output")
		defer os.RemoveAll(outName)

		cmd := &pull.Command{
			UI: &cli.BasicUi{
				Writer:      out,
				ErrorWriter: bytes.NewBuffer(nil),
			},
			Server: nitrousSrv.URL(),
			Client: httpclient.New("correct_token"),
		}
		cmd.Run([]string{"myhost", "scratch"})

		// assert that the temp file contain parsed output, not the json stream
		output, _ := ioutil.ReadFile(outName)
		Expect(string(output)).To(ContainSubstring("Pulling image 'scratch' to 'myhost'..."))
		Expect(string(output)).To(ContainSubstring("Pulling..."))
	})

	Context("when given wrong arguments", func() {
		It("prints usage and exits with error", func() {
			stdout := bytes.NewBuffer(nil)
			cmd := &pull.Command{UI: &cli.BasicUi{Writer: stdout}}
			code := cmd.Run([]string{})
			Expect(stdout.String()).To(ContainSubstring("Usage:"))
			Expect(code).To(Equal(1))
		})
	})

	Context("Help", func() {
		It("outputs usage", func() {
			cmd := &pull.Command{}
			Expect(cmd.Help()).To(ContainSubstring("Usage: nitro images:pull"))
		})
	})
})

func setupNitrous(host string, statusCode int, response string) *ghttp.Server {
	// Setup handlers for nitrous server (hosts)
	handler := ghttp.CombineHandlers(
		ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
		ghttp.VerifyRequest("POST", "/api/v0/hosts/"+host+"/images/pull"),
		ghttp.RespondWith(statusCode, response),
	)
	srv := ghttp.NewServer()
	srv.AppendHandlers(handler)
	return srv
}
