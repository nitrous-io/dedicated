package hosts_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/hosts"
	"github.com/nitrous-io/nitro/test"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hosts")
}

var _ = Describe("hosts", func() {
	var (
		cmd *hosts.Command
		ui  *cli.MockUi
	)

	BeforeEach(func() {
		ui = &cli.MockUi{}
		cmd = &hosts.Command{
			UI:     ui,
			Server: "example.com",
			Client: httpclient.New("a_token"),
		}
	})

	Context("hitting the server", func() {
		It("populates request correctly", func() {
			ms := test.RequestChecker(func(r *http.Request) {
				Expect(r.Method).To(Equal("GET"))
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer a_token"))
			})
			ms.Check(func() (*http.Response, error) {
				cmd.Server = ms.URL
				cmd.Run([]string{})
				return nil, nil
			})
		})

		Context("when server responds with hosts", func() {
			var (
				responseJSON = strings.TrimSpace(`
				{"hosts":[{"ID":1,"UserID":1,"RegionID":1,"PlanID":1,"ProviderTemplateID":1,"Name":"host-1","LongName":{"String":"","Valid":false},"State":"born","IPAddress":{"String":"","Valid":false},"ProviderIdentifier":{"String":"","Valid":false},"MaxContainers":{"Int64":0,"Valid":false},"FreighterdPort":{"Int64":0,"Valid":false},"TerminatedAt":null,"CreatedAt":"2014-07-10T12:05:14.473679Z","UpdatedAt":"2014-07-10T12:05:14.473679Z"},{"ID":2,"UserID":1,"RegionID":2,"PlanID":1,"ProviderTemplateID":2,"Name":"host-2","LongName":{"String":"","Valid":false},"State":"running","IPAddress":{"String":"","Valid":false},"ProviderIdentifier":{"String":"","Valid":false},"MaxContainers":{"Int64":0,"Valid":false},"FreighterdPort":{"Int64":0,"Valid":false},"TerminatedAt":null,"CreatedAt":"2014-07-10T12:05:14.473679Z","UpdatedAt":"2014-07-10T12:05:14.473679Z"}]}
	`)

				formattedOutput = strings.TrimSpace(`
Name      State
host-1    born
host-2    running
`) + "\n\n"
			)

			It("outputs formatted hosts on success", func() {
				ms := test.QuickMockServer("/api/v0/hosts", http.StatusOK,
					responseJSON)
				defer ms.Close()
				cmd.Server = ms.URL
				code := cmd.Run([]string{})
				Expect(ui.OutputWriter.String()).To(Equal(formattedOutput))
				Expect(code).To(Equal(0))
			})
		})

		Context("when user has no hosts", func() {
			It("displays no hosts found", func() {
				ms := test.QuickMockServer("/api/v0/hosts", http.StatusOK,
					`{"hosts":[],"status":"ok"}`)
				defer ms.Close()
				cmd.Server = ms.URL
				code := cmd.Run([]string{})
				Expect(ui.OutputWriter.String()).To(Equal("No hosts found\n"))
				Expect(code).To(Equal(0))
			})
		})

		Context("when server responds with an internal_server_error", func() {
			It("displays unknown error message", func() {
				ms := test.QuickMockServer("/api/v0/hosts", http.StatusInternalServerError,
					`{"error":"internal_server_error","error_hash":"b482fd5b602650c798366fe9ee8e6cf65ef759fc"}`,
				)
				defer ms.Close()
				cmd.Server = ms.URL
				code := cmd.Run([]string{})
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("Unknown error occurred"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
