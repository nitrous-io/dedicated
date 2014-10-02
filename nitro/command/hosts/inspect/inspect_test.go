package inspect_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/hosts/inspect"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "hosts/inspect")
}

var _ = Describe("hosts/inspect", func() {
	var (
		cmd *inspect.Command
		ui  *cli.MockUi

		srv            *ghttp.Server
		statusCodeR1   int
		responseBodyR1 string
		statusCodeR2   int
		responseBodyR2 string
	)

	BeforeEach(func() {
		ui = &cli.MockUi{}

		srv = ghttp.NewServer()
		// 1st request: get host.
		srv.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("GET", "/api/v0/hosts/foo"),
			ghttp.RespondWithPtr(&statusCodeR1, &responseBodyR1),
		))
		// 2nd request: get host stats.
		srv.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("GET", "/api/v0/hosts/foo/stats"),
			ghttp.RespondWithPtr(&statusCodeR2, &responseBodyR2),
		))

		cmd = &inspect.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
	})

	Context("when the API responds with the host and host stats", func() {
		BeforeEach(func() {
			statusCodeR1 = http.StatusOK
			responseBodyR1 = `{
				"host": {
					"id": 58,
					"slug": "foo",
					"ip_address": "127.0.0.1",
					"freighterd_port": 49153,
					"size": "micro",
					"state": "running",
					"hostname": "foo.nitrouspro.com"
				}
			}`
			statusCodeR2 = http.StatusOK
			responseBodyR2 = `{
				"stats": {
					"uptime": 107210,
					"memory_usage": 1896906752,
					"memory_total": 2100002816,
					"storage_usage": 19692343296,
					"storage_total": 83366748160,
					"num_cpu": 2,
					"per_cpu_usage": [0.5, 20.3]
				}
			}`
		})

		It("prints the host", func() {
			code := cmd.Run([]string{"foo"})
			Expect(srv.ReceivedRequests()).To(HaveLen(2))
			Expect(code).To(Equal(0))
			Expect(ui.OutputWriter.String()).To(ContainSubstring(strings.TrimSpace(`
Slug  State    Size   IP         Hostname
foo   running  micro  127.0.0.1  foo.nitrouspro.com
`)))
			Expect(ui.OutputWriter.String()).To(ContainSubstring(strings.TrimSpace(`
Uptime  Memory Used (bytes)  Memory Total (bytes)  Storage Used (bytes)  Storage Total (bytes)  Number of CPUs  CPU 0 Usage  CPU 1 Usage
107210  1896906752           2100002816            19692343296           83366748160            2               0.50 %       20.30 %
`)))
		})

		Context("when --json flag is set", func() {
			It("prints the host in JSON format", func() {
				code := cmd.Run([]string{"foo", "--json"})
				Expect(code).To(Equal(0))
				Expect(ui.OutputWriter.String()).To(MatchJSON(`{
					"host": {
						"slug": "foo",
						"ip_address": "127.0.0.1",
						"size": "micro",
						"state": "running",
						"hostname": "foo.nitrouspro.com"
					},
					"stats": {
						"uptime": 107210,
						"memory_usage": 1896906752,
						"memory_total": 2100002816,
						"storage_usage": 19692343296,
						"storage_total": 83366748160,
						"num_cpu": 2,
						"per_cpu_usage": [0.5, 20.3]
					}
				}`))
			})
		})

		Context("when API responds with error to request for host stats", func() {
			BeforeEach(func() {
				statusCodeR2 = http.StatusBadRequest
				responseBodyR2 = `{"error":"invalid_request","error_description":"host not running"}`
			})

			It("prints the host without stats", func() {
				code := cmd.Run([]string{"foo"})
				Expect(srv.ReceivedRequests()).To(HaveLen(2))
				Expect(code).To(Equal(0))
				Expect(ui.OutputWriter.String()).To(ContainSubstring(strings.TrimSpace(`
Slug  State    Size   IP         Hostname
foo   running  micro  127.0.0.1  foo.nitrouspro.com
`)))
				Expect(ui.OutputWriter.String()).NotTo(ContainSubstring("Uptime"))
			})

			Context("when --json flag is set", func() {
				It("prints the host in JSON format", func() {
					code := cmd.Run([]string{"foo", "--json"})
					Expect(code).To(Equal(0))
					Expect(ui.OutputWriter.String()).To(MatchJSON(`{
						"host": {
							"slug": "foo",
							"ip_address": "127.0.0.1",
							"size": "micro",
							"state": "running",
							"hostname": "foo.nitrouspro.com"
						}
					}`))
				})
			})
		})
	})

	Context("when not given any arguments", func() {
		It("prints usage and exits with error", func() {
			code := cmd.Run([]string{})
			Expect(code).To(Equal(1))
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
		})
	})

	Context("when given > 1 argument", func() {
		It("prints usage and exits with error", func() {
			code := cmd.Run([]string{"foo", "you", "too"})
			Expect(code).To(Equal(1))
			Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
		})
	})
})
