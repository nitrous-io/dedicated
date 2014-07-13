package hostscreate_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/hostscreate"
	"github.com/nitrous-io/nitro/test"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hostscreate")
}

var _ = Describe("hosts:create", func() {
	var (
		cmd *hostscreate.Command
		ui  *cli.MockUi
	)

	BeforeEach(func() {
		ui = &cli.MockUi{}
		cmd = &hostscreate.Command{
			UI:     ui,
			Server: "example.com",
			Client: httpclient.New("a_token"),
		}
	})

	It("requires size as an argument", func() {
		code := cmd.Run([]string{})
		Expect(code).To(Equal(1))
		Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
	})

	Context("hitting the server", func() {
		doCmd := func(ms *test.MockServer) int {
			cmd.Server = ms.URL
			return cmd.Run([]string{"micro",
				"-r", "apse",
				"-n", "foo",
				"-L", "foobar",
			})
		}

		It("populates request correctly", func() {
			ms := test.RequestChecker(func(r *http.Request) {
				Expect(r.Method).To(Equal("POST"))
				Expect(r.FormValue("size")).To(Equal("micro"))
				Expect(r.FormValue("region")).To(Equal("apse"))
				Expect(r.FormValue("name")).To(Equal("foo"))
				Expect(r.FormValue("longname")).To(Equal("foobar"))
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer a_token"))
			})

			ms.Check(func() (*http.Response, error) {
				doCmd(ms)
				return nil, nil
			})
		})

		Context("when request succeeds", func() {
			It("reports success", func() {
				ms := test.QuickMockServer("/api/v0/hosts", http.StatusCreated,
					`{"status":"created"}`)
				defer ms.Close()

				code := doCmd(ms)
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Done"))
				Expect(code).To(Equal(0))
			})
		})

		Context("when request fails", func() {
			It("displays error message", func() {
				ms := test.QuickMockServer("/api/v0/hosts", http.StatusBadRequest,
					test.ErrorJSON("validation_failed", "region not found"))
				defer ms.Close()

				code := doCmd(ms)
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("region not found"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
