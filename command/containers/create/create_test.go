package create_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/containers/create"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "containers/create")
}

var _ = Describe("containers/create", func() {
	var (
		cmd      *create.Command
		outfile  *os.File
		errorBuf *bytes.Buffer

		slug = "foo"

		srv                *ghttp.Server
		pullStatusCode     = http.StatusOK
		pullResponseBody   = ""
		createStatusCode   = http.StatusCreated
		createResponseBody = ""
		startStatusCode    = http.StatusOK
		startResponseBody  = ""
	)

	BeforeEach(func() {
		var err error
		srv = ghttp.NewServer()

		// Setup handlers: pull image.
		srv.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer a_token"),
			ghttp.VerifyRequest("POST", "/api/v0/hosts/host-1/images/pull"),
			func(w http.ResponseWriter, req *http.Request) {
				req.ParseForm()
				Expect(req.PostFormValue("update")).To(Equal("false"))
			},
			ghttp.RespondWithPtr(&pullStatusCode, &pullResponseBody),
		))

		// Setup handlers: create container.
		srv.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer a_token"),
			ghttp.VerifyRequest("POST", "/api/v0/containers"),
			func(w http.ResponseWriter, req *http.Request) {
				req.ParseForm()
				Expect(req.PostFormValue("host")).To(Equal("host-1"))
			},
			ghttp.RespondWithPtr(&createStatusCode, &createResponseBody),
		))

		// Setup handlers: start container.
		srv.AppendHandlers(ghttp.CombineHandlers(
			ghttp.VerifyHeaderKV("Authorization", "Bearer a_token"),
			ghttp.VerifyRequest("POST", "/api/v0/containers/"+slug+"/start"),
			func(w http.ResponseWriter, req *http.Request) {
				req.ParseForm()
			},
			ghttp.RespondWithPtr(&startStatusCode, &startResponseBody),
		))

		// Create a tmp file to store output.
		outfile, err = ioutil.TempFile("/tmp", "images-pull")
		Expect(err).To(BeNil(), "failed to create tmp file for storing output")

		errorBuf = bytes.NewBuffer(nil)
		cmd = &create.Command{
			UI: &cli.BasicUi{
				Writer:      outfile,
				ErrorWriter: errorBuf,
			},
			Server: srv.URL(),
			Client: httpclient.New("a_token"),
		}
	})

	AfterEach(func() {
		srv.Close()
		os.RemoveAll(outfile.Name())
	})

	Context("when not all options are specified", func() {
		It("populates request with only given params", func() {
			cmd.Run([]string{
				"host-1",
				"--slug=" + slug,
			})
			reqs := srv.ReceivedRequests()
			Expect(reqs).To(HaveLen(2))
			Expect(reqs[1].PostFormValue("slug")).To(Equal(slug))
		})
	})

	Context("when slug is not specified", func() {
		It("populates request with a randomly generated slug", func() {
			cmd.Run([]string{
				"host-1",
			})
			reqs := srv.ReceivedRequests()
			Expect(reqs).To(HaveLen(2))
			Expect(reqs[1].PostFormValue("slug")).NotTo(Equal(""))
		})
	})

	Context("when host is not specified", func() {
		doCmd := func() int {
			return cmd.Run([]string{
				"--slug=" + slug,
				"--image=ubuntu",
			})
		}

		It("returns an error message without making a request", func() {
			code := doCmd()
			Expect(errorBuf.String()).To(ContainSubstring("Please specify a host"))
			Expect(code).To(Equal(1))
			Expect(srv.ReceivedRequests()).To(HaveLen(0))
		})
	})

	Context("when all options are provided", func() {
		doCmd := func() int {
			return cmd.Run([]string{
				"host-1",
				"--slug=" + slug,
				"--image=ubuntu",
				"--env=PAGER=most",
				"--volume=/shared", // repeat args supported
				"--volume=/music",
				"--port-forward=8080:3000,127.0.0.1:4242", // comma-separated supported
			})
		}

		It("populates the pull image request's params", func() {
			doCmd()
			reqs := srv.ReceivedRequests()
			Expect(reqs).To(HaveLen(2))
			r := reqs[0]
			Expect(r.PostFormValue("repository")).To(Equal("ubuntu"))
		})

		It("populates the create request's params", func() {
			doCmd()
			reqs := srv.ReceivedRequests()
			Expect(reqs).To(HaveLen(2))
			r := reqs[1]
			Expect(r.PostFormValue("slug")).To(Equal(slug))
			Expect(r.PostFormValue("image")).To(Equal("ubuntu"))
			Expect(r.PostForm["env[]"]).To(Equal([]string{"PAGER=most"}))
			Expect(r.PostForm["volume[]"]).To(Equal([]string{"/shared", "/music"}))
			Expect(r.PostForm["port[]"]).To(Equal([]string{"8080:3000", "127.0.0.1:4242"}))
		})

		Context("when the create request succeeds", func() {
			BeforeEach(func() {
				createResponseBody = `{
					"status": "created",
					"container": { "slug": "` + slug + `", "image": "ubuntu", "state": "stopped" }
				}`
			})

			Context("when the start request succeeds", func() {
				BeforeEach(func() {
					startResponseBody = `{
						"container": { "slug": "` + slug + `", "image": "ubuntu", "state": "running" }
					}`
				})

				It("populates the start request's params", func() {
					code := doCmd()
					Expect(code).To(Equal(0))

					reqs := srv.ReceivedRequests()
					Expect(reqs).To(HaveLen(3))
					r := reqs[2]
					Expect(r.PostForm["env[]"]).To(Equal([]string{"PAGER=most"}))
					Expect(r.PostForm["volume[]"]).To(Equal([]string{"/shared", "/music"}))
					Expect(r.PostForm["port[]"]).To(Equal([]string{"8080:3000", "127.0.0.1:4242"}))
				})

				It("prints the container's slug", func() {
					code := doCmd()
					Expect(code).To(Equal(0))

					output, _ := ioutil.ReadFile(outfile.Name())
					Expect(output).To(ContainSubstring("Created new container: '" + slug + "'"))
				})
			})

			Context("when the start request fails", func() {
				BeforeEach(func() {
					startStatusCode = http.StatusInternalServerError
				})

				It("prints the container", func() {
					code := doCmd()
					Expect(code).To(Equal(1))

					output, _ := ioutil.ReadFile(outfile.Name())
					Expect(errorBuf).To(ContainSubstring("Container created but failed to start"))
					Expect(output).To(ContainSubstring(slug))
				})
			})
		})

		Context("when the create request fails", func() {
			BeforeEach(func() {
				createStatusCode = http.StatusBadRequest
				createResponseBody = `{"error":"validation_failed","error_description":"slug invalid"}`
			})

			It("displays error message", func() {
				code := doCmd()
				Expect(errorBuf.String()).To(ContainSubstring("slug invalid"))
				Expect(code).To(Equal(1))
			})
		})
	})
})
