package ibuild_test

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/nitrous-io/nitro/command"
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
		ui           *command.MockUI
		srv          *ghttp.Server
		handlers     http.HandlerFunc
		statusCode   int
		responseBody string
		tmpdir       string
		expectedName string
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
		ui = &command.MockUI{&cli.MockUi{}}
		srv = ghttp.NewServer()
		expectedName = ""
		handlers = ghttp.CombineHandlers(
			func(w http.ResponseWriter, req *http.Request) {
				if expectedName != "" {
					Expect(req.FormValue("name")).To(Equal(expectedName))
				} else {
					Expect(req.FormValue("name")).NotTo(BeEmpty())
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
		cmd = &ibuild.Command{
			UI:     ui,
			Server: srv.URL(),
			Client: httpclient.New("correct_token"),
		}
	})

	AfterEach(func() {
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
				responseBody = `{"status":"OK"}`
			})

			It("uploads Dockerfile inside tar", func() {
				fpath := makeDockerfile("")
				code := doCmd(fpath)
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Built image"))
				Expect(code).To(Equal(0))
			})
		})

		Context("successful build with tarball", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `{"status":"OK"}`
			})

			It("uploads tarball", func() {
				fpath := makeValidTar()
				code := doCmd(fpath)
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Built image"))
				Expect(code).To(Equal(0))
			})
		})

		Context("with name specified", func() {
			BeforeEach(func() {
				statusCode = http.StatusOK
				responseBody = `{"status":"OK"}`
				expectedName = "UseThisName"
			})

			It("uses name", func() {
				fpath := makeDockerfile("")
				code := cmd.Run([]string{"myhost", fpath, "--tag=UseThisName"})
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Built image"))
				Expect(code).To(Equal(0))
			})
		})

		Context("error on build context not existing", func() {
			It("outputs error message", func() {
				code := doCmd("/tmp/nosuch")
				Expect(ui.ErrorWriter.String()).To(ContainSubstring("no such file"))
				Expect(code).To(Equal(1))
			})
		})

		Context("JSON response with error", func() {
			BeforeEach(func() {
				statusCode = http.StatusBadRequest
				responseBody = `{"error": "validation_failed", "error_description": "image not found"}`
			})

			It("outputs error message", func() {
				fpath := makeDockerfile("")
				code := doCmd(fpath)
				Expect(ui.ErrorWriter.String()).To(Equal("image not found\n"))
				Expect(code).To(Equal(1))
			})
		})
	})

	Context("when given wrong arguments", func() {
		It("prints usage and exits with error", func() {
			for _, t := range [][]string{[]string{}, []string{"hostonly"}, []string{"host", "buildcontext", "somerandomextrastuff"}} {
				code := cmd.Run(t)
				Expect(ui.OutputWriter.String()).To(ContainSubstring("Usage:"))
				Expect(code).To(Equal(1))
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
