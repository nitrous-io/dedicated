package list_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/images/list"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "images/list")
}

var _ = Describe("images/list", func() {

	Describe("Help", func() {
		var (
			cmd *list.Command
		)

		BeforeEach(func() {
			cmd = &list.Command{}
		})

		It("should expose the usage", func() {
			Expect(cmd.Help()).To(ContainSubstring("Usage: nitro images <host>"))
		})
	})

	Describe("Run", func() {
		var (
			cmd *list.Command
			ui  *cli.MockUi
		)

		BeforeEach(func() {
			ui = &cli.MockUi{}

			cmd = &list.Command{
				UI: ui,
			}
		})

		Context("when host is not specified", func() {
			It("prints the Help output", func() {
				code := cmd.Run([]string{})
				Expect(ui.OutputWriter.String()).To(ContainSubstring(cmd.Help()))
				Expect(code).To(Equal(1))
			})
		})

		Context("when host is specified", func() {
			var (
				svr          *ghttp.Server
				handlers     http.HandlerFunc
				statusCode   int
				responseBody string

				host = "a_host"
			)

			setupServer := func(path string) {
				ui = &cli.MockUi{}

				svr = ghttp.NewServer()
				handlers = ghttp.CombineHandlers(
					ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
					ghttp.VerifyRequest("GET", path),
					ghttp.RespondWithPtr(&statusCode, &responseBody),
				)
				svr.AppendHandlers(handlers)

				cmd = &list.Command{
					UI:     ui,
					Server: svr.URL(),
					Client: httpclient.New("correct_token"),
				}
			}

			teardownServer := func() {
				if svr != nil {
					svr.Close()
				}
			}

			BeforeEach(func() {
				setupServer(fmt.Sprintf("/api/v0/hosts/%s/images", host))
			})

			AfterEach(func() {
				teardownServer()
			})

			Context("when the response is successful", func() {
				BeforeEach(func() {
					statusCode = http.StatusOK
				})

				Context("with no images", func() {
					BeforeEach(func() {
						responseBody = `{"images": []}`
					})

					It("should communicate there are no Images found", func() {
						code := cmd.Run([]string{host})
						Expect(ui.OutputWriter.String()).To(ContainSubstring("No images found."))
						Expect(code).To(Equal(0))
					})
				})

				Context("with images", func() {
					BeforeEach(func() {
						responseBody = `{
							"images": [
								{
									"id": "12",
									"slug": "source1",
									"tag": "sourcetag1",
									"name": "source1 img",
									"created_at": "2013-11-01T00:00:00Z",
									"actual_size": 12,
									"virtual_size": 39
								},
								{
									"id": "13",
									"slug": "source2",
									"tag": "sourcetag2",
									"name": "source2 img",
									"created_at": "2013-11-02T00:00:00Z",
									"actual_size": 10,
									"virtual_size": 48
								}
							]
						}`
					})

					It("should communicate list the images", func() {
						code := cmd.Run([]string{host})
						Expect(ui.OutputWriter.String()).To(ContainSubstring(strings.TrimSpace(`
ID    Slug       Tag           Name           Created                          Size   Virtual Size
12    source1    sourcetag1    source1 img    2013-11-01 00:00:00 +0000 UTC    12     39
13    source2    sourcetag2    source2 img    2013-11-02 00:00:00 +0000 UTC    10     48
`)))
						Expect(code).To(Equal(0))
					})
				})
			})

			Context("when the response does not respond with OK", func() {
				BeforeEach(func() {
					statusCode = http.StatusBadRequest
				})

				Context("with no error_description", func() {
					BeforeEach(func() {
						responseBody = `{"foo": "bar"}`
					})

					It("should exit 1 with a general error", func() {
						code := cmd.Run([]string{host})
						Expect(ui.ErrorWriter.String()).To(ContainSubstring("Unknown error occurred"))
						Expect(code).To(Equal(1))
					})
				})

				Context("with a error_description", func() {
					BeforeEach(func() {
						responseBody = `{"error_description": "Bad weather, cannot connect to cloud."}`
					})

					It("should exit 1 with the error descrption", func() {
						code := cmd.Run([]string{host})
						Expect(ui.ErrorWriter.String()).To(ContainSubstring("Bad weather, cannot connect to cloud."))
						Expect(code).To(Equal(1))
					})
				})
			})
		})
	})
})
