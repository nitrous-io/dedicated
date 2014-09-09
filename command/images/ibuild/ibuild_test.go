package ibuild_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command/images/ibuild"
	"github.com/nitrous-io/nitrous-common/httpclient"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "images/ibuild")
}

var _ = Describe("images/ibuild", func() {
	var (
		cmd          *ibuild.Command
		errorWriter  *bytes.Buffer
		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string
		tmpdir       string
		outfile      *os.File
		expectedSlug string
		dfBytes      = []byte("FROM scratch")
	)

	makeDockerfile := func(dfPath string) string {
		if dfPath == "" {
			dfPath = path.Join(tmpdir, "Dockerfile")
		}
		err := ioutil.WriteFile(dfPath, dfBytes, 0600)
		Expect(err).To(BeNil())
		return dfPath
	}

	// creates and closes tmp file, returns filename, caller to remove file
	makeInvalidTar := func() string {
		f, err := ioutil.TempFile(tmpdir, "build-invalid-tar")
		Expect(err).To(BeNil())
		defer f.Close()
		_, err = f.Write([]byte("not tar"))
		Expect(err).To(BeNil())
		return f.Name()
	}

	// creates and closes tmp file, returns filename, caller to remove file
	makeValidTar := func() string {
		// building a tar with Go here would be replicating dockerfileTar, so
		// use tar command to build
		f, err := ioutil.TempFile(tmpdir, "build-valid-tar")
		Expect(err).To(BeNil())
		f.Close()

		dfPath := makeDockerfile("")
		cmd := exec.Command("tar", "cf", f.Name(), path.Base(dfPath))
		cmd.Dir = tmpdir
		err = cmd.Run()
		Expect(err).To(BeNil())
		return f.Name()
	}

	BeforeEach(func() {
		var err error
		tmpdir, err = ioutil.TempDir("", "nitro-build")
		Expect(err).To(BeNil())
		srv = ghttp.NewServer()
		expectedSlug = ""
		handlers = ghttp.CombineHandlers(
			func(w http.ResponseWriter, req *http.Request) {
				if expectedSlug != "" {
					Expect(req.FormValue("slug")).To(Equal(expectedSlug))
				} else {
					Expect(req.FormValue("slug")).NotTo(BeEmpty())
				}
			},
			func(w http.ResponseWriter, req *http.Request) {
				// check that request sends tarball in desired format
				// in 'file' param
				f, _, err := req.FormFile("file")
				Expect(err).To(BeNil())
				defer f.Close()
				filename := make([]byte, 100) // tar header filename is 0-100
				_, err = f.Read(filename)
				Expect(err).To(BeNil())
				Expect(string(bytes.Trim(filename, "\x00"))).To(Equal("Dockerfile"))
				f.Seek(512, 0) // tar file header is 512 bytes
				buf := make([]byte, len(dfBytes))
				_, err = f.Read(buf)
				Expect(err).To(BeNil())
				Expect(buf).To(Equal(dfBytes))
			},
			ghttp.VerifyHeaderKV("Authorization", "Bearer correct_token"),
			ghttp.VerifyRequest("POST", "/api/v0/hosts/myhost/images"),
			ghttp.RespondWithPtr(&statusCode, &responseBody),
		)
		srv.AppendHandlers(handlers)

		// Create a tmp file to store build output which normally gets written
		// to stdout.
		outfile, err = ioutil.TempFile(tmpdir, "output")
		Expect(err).To(BeNil())

		errorWriter = bytes.NewBuffer(nil)
		cmd = &ibuild.Command{
			UI: &cli.BasicUi{
				Writer:      outfile,
				ErrorWriter: errorWriter,
			},
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}
	})

	AfterEach(func() {
		outfile.Close()
		if strings.HasPrefix(tmpdir, os.TempDir()) {
			os.RemoveAll(tmpdir)
		}
		srv.Close()
	})

	Context("nitro images:build host buildcontext", func() {
		doCmd := func(fpath string) int {
			return cmd.Run([]string{"myhost", fpath})
		}

		Context("successful build with Dockerfile", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `
					{"stream":"Step 0 : FROM ubuntu:latest\n"}
					{"stream":" ---\u003e 4300eb9d3c8d\n"}
					{"stream":"Step 1 : MAINTAINER docker <eng@docker.com>\n"}
					{"stream":" ---\u003e Using cache\n"}
					{"stream":" ---\u003e 3a3ed758c370\n"}
					{"stream":"Step 2 : CMD /usr/bin/top\n"}
					{"stream":" ---\u003e Running in 36b1479cc2e4\n"}
					{"stream":" ---\u003e 4b6188aebe39\n"}
					{"stream":"Removing intermediate container 36b1479cc2e4\n"}
					{"stream":"Successfully built 4b6188aebe39\n"}
			    `
			})

			It("uploads Dockerfile inside tar", func() {
				fpath := makeDockerfile("")
				code := doCmd(fpath)
				Expect(code).To(Equal(0))
				Expect(srv.ReceivedRequests()).To(HaveLen(1))
				output, _ := ioutil.ReadFile(outfile.Name())
				Expect(string(output)).To(ContainSubstring("Step 0 : FROM ubuntu:latest"))
				Expect(string(output)).To(ContainSubstring("Step 1 : MAINTAINER docker <eng@docker.com>"))
				Expect(string(output)).To(ContainSubstring("Successfully built 4b6188aebe39"))

				Expect(string(output)).To(ContainSubstring("Built image"))
			})
		})

		Context("successful build with tarball", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `
					{"stream":"Step 0 : FROM ubuntu:latest\n"}
					{"stream":" ---\u003e 4300eb9d3c8d\n"}
					{"stream":"Step 1 : MAINTAINER docker <eng@docker.com>\n"}
					{"stream":" ---\u003e Using cache\n"}
					{"stream":" ---\u003e 3a3ed758c370\n"}
					{"stream":"Step 2 : CMD /usr/bin/top\n"}
					{"stream":" ---\u003e Running in 36b1479cc2e4\n"}
					{"stream":" ---\u003e 4b6188aebe39\n"}
					{"stream":"Removing intermediate container 36b1479cc2e4\n"}
					{"stream":"Successfully built 4b6188aebe39\n"}
			    `
			})

			It("uploads tarball", func() {
				fpath := makeValidTar()
				code := doCmd(fpath)
				Expect(code).To(Equal(0))
				Expect(srv.ReceivedRequests()).To(HaveLen(1))
				output, _ := ioutil.ReadFile(outfile.Name())
				Expect(string(output)).To(ContainSubstring("Built image"))
			})
		})

		Context("with slug specified", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `
					{"stream":"Step 0 : FROM ubuntu:latest\n"}
					{"stream":" ---\u003e 4300eb9d3c8d\n"}
					{"stream":"Step 1 : MAINTAINER docker <eng@docker.com>\n"}
					{"stream":" ---\u003e Using cache\n"}
					{"stream":" ---\u003e 3a3ed758c370\n"}
					{"stream":"Step 2 : CMD /usr/bin/top\n"}
					{"stream":" ---\u003e Running in 36b1479cc2e4\n"}
					{"stream":" ---\u003e 4b6188aebe39\n"}
					{"stream":"Removing intermediate container 36b1479cc2e4\n"}
					{"stream":"Successfully built 4b6188aebe39\n"}
			    `
				expectedSlug = "UseThisSlug"
			})

			It("uses slug", func() {
				fpath := makeDockerfile("")
				code := cmd.Run([]string{"myhost", fpath, "--slug=UseThisSlug"})
				Expect(code).To(Equal(0))
				Expect(srv.ReceivedRequests()).To(HaveLen(1))
				output, _ := ioutil.ReadFile(outfile.Name())
				Expect(string(output)).To(ContainSubstring("Built image"))
			})
		})

		Context("error on build context not existing", func() {
			It("outputs error message", func() {
				code := doCmd("/tmp/nosuch")
				Expect(code).To(Equal(1))
				Expect(srv.ReceivedRequests()).To(HaveLen(0))
				Expect(errorWriter.String()).To(ContainSubstring("no such file or directory"))
			})
		})

		Context("JSON response with error", func() {
			BeforeEach(func() {
				statusCode = http.StatusInternalServerError
				responseBody = `
					{"stream":"Step 0 : FROM broken\n"}
					{"stream":" ---\u003e 4300eb9d3c8d\n"}
					{"error":"Error...", "errorDetail":{"code": 123, "message": "Error pulling 'broken'"}}
			    `
			})

			It("outputs error message", func() {
				fpath := makeDockerfile("")
				code := doCmd(fpath)
				Expect(code).To(Equal(1))
				Expect(srv.ReceivedRequests()).To(HaveLen(1))
				output, _ := ioutil.ReadFile(outfile.Name())
				Expect(string(output)).NotTo(ContainSubstring("Step 0 : FROM broken")) // Stream is not displayed
				Expect(errorWriter.String()).NotTo(BeEmpty())
			})
		})
	})

	Context("when given wrong arguments", func() {
		It("prints usage and exits with error", func() {
			for _, t := range [][]string{{}, {"hostonly"}, {"host", "buildcontext", "somerandomextrastuff"}} {
				code := cmd.Run(t)
				Expect(code).To(Equal(1))
				output, _ := ioutil.ReadFile(outfile.Name())
				Expect(string(output)).To(ContainSubstring("Usage:"))
			}
		})
	})

	Context("Help", func() {
		It("outputs usage", func() {
			Expect(cmd.Help()).To(ContainSubstring("Usage: nitro images:build"))
		})
	})

	Context("isValidTar()", func() {
		It("is true with valid tarfile", func() {
			path := makeValidTar()
			f, err := os.Open(path)
			Expect(err).To(BeNil())
			defer f.Close()
			Expect(ibuild.IsValidTar(f)).To(BeTrue())
		})

		It("is true with valid tarfile that contains a './Dockerfile' in the archive root", func() {
			tarpath := path.Join(tmpdir, "valid.tar")
			// Create a tarball using "tar cf file.tar ./*".
			// This results in a tarball listing with name "./Dockerfile"
			makeDockerfile(path.Join(tmpdir, "Dockerfile"))
			cmd := exec.Command("sh", "-c", fmt.Sprintf("tar cf %s ./*", tarpath))
			cmd.Dir = tmpdir
			err := cmd.Run()
			Expect(err).To(BeNil())

			f, err := os.Open(tarpath)
			Expect(err).To(BeNil())
			defer f.Close()
			Expect(ibuild.IsValidTar(f)).To(BeTrue())
		})

		It("is false with invalid tarfile", func() {
			path := makeInvalidTar()
			f, err := os.Open(path)
			Expect(err).To(BeNil())
			defer f.Close()
			Expect(ibuild.IsValidTar(f)).NotTo(BeTrue())
		})
	})

	Context("dockerfileTar()", func() {
		It("produces valid tarfile", func() {
			dfPath := makeDockerfile("")
			f, err := os.Open(dfPath)
			Expect(err).To(BeNil())
			defer f.Close()
			r, err := ibuild.DockerfileTar(f)
			Expect(err).To(BeNil())
			Expect(ibuild.IsValidTar(r)).To(BeTrue())
		})
	})
})
