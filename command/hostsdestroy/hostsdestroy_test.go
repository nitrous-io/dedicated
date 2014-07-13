package hostsdestroy_test

import (
	"net/http"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/hostsdestroy"
	"github.com/nitrous-io/nitro/test"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hostsdestroy")
}

var _ = Describe("hosts:destroy", func() {
	var (
		cmd *hostsdestroy.Command
		ui  *cli.MockUi
	)

	BeforeEach(func() {
		ui = &cli.MockUi{}
		cmd = &hostsdestroy.Command{
			UI:     ui,
			Server: "example.com",
			Client: httpclient.New("a_token"),
		}
	})

	It("requires name as an argument", func() {
		code := cmd.Run([]string{})
		Expect(code).To(Equal(1))
		Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
	})

	Context("hitting the server", func() {
		It("populates request correctly", func() {
			ms := test.RequestChecker(func(r *http.Request) {
				Expect(r.Method).To(Equal("DELETE"))
				Expect(r.URL.Path).To(Equal("/api/v0/hosts/foo"))
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer a_token"))
			})

			ms.Check(func() (*http.Response, error) {
				cmd.Server = ms.URL
				cmd.Run([]string{"foo"})
				return nil, nil
			})
		})

		Context("when request succeeds", func() {
			It("reports success", func() {
				ms := test.QuickMockServer("/api/v0/hosts/foo", http.StatusOK, `{"status":"ok"}`)
				defer ms.Close()

				cmd.Server = ms.URL
				code := cmd.Run([]string{"foo"})

				Expect(ui.OutputWriter.String()).To(ContainSubstring("Done"))
				Expect(code).To(Equal(0))
			})
		})

		Context("when request fails", func() {
			It("displays error message", func() {
				ms := test.QuickMockServer("/api/v0/hosts/foo", http.StatusBadRequest,
					test.ErrorJSON("invalid_request", "host not found"))
				defer ms.Close()

				cmd.Server = ms.URL
				code := cmd.Run([]string{"foo"})

				Expect(ui.ErrorWriter.String()).To(ContainSubstring("host not found"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
