package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("BuildImage", func() {
	getMatchingImage := func(prov *docker.DockerProvider) *provider.ImageInfo {
		allImages, err := prov.ListImages()
		Expect(err).To(BeNil())

		for _, image := range allImages {
			if image.Name == "myimage" && image.Tag == "latest" {
				return image
			}
		}
		return nil
	}

	itCanRunBuiltImage := func(id string) {
		res, err := exec.Command("docker", "run", "--rm=true", id).Output()
		Expect(err).To(BeNil())
		Expect(strings.TrimSpace(string(res))).To(Equal("Hello from Docker!"))
	}

	var prov *docker.DockerProvider
	BeforeEach(func() {
		data, err := ioutil.TempDir("", "freightertest")
		Expect(err).To(BeNil(), "failed to create temp dir")
		prov, err = docker.NewDockerProvider(data)
		Expect(err).To(BeNil(), "failed to initialize docker provider")
	})

	Context("when build context is provided as a tarball", func() {
		It("creates an Image successfully and correctly stores the additional data", func() {
			buf := new(bytes.Buffer)
			gzipWriter := gzip.NewWriter(buf)
			tarWriter := tar.NewWriter(gzipWriter)

			dockerSource := `
				FROM busybox:latest
				MAINTAINER Nitrous.IO <hello@nitrous.io>
				ADD some_file.txt /some/file.txt
				CMD cat /some/file.txt
			`

			err := tarWriter.WriteHeader(&tar.Header{
				Name: "Dockerfile",
				Size: int64(len(dockerSource)),
			})
			Expect(err).To(BeNil())
			_, err = tarWriter.Write([]byte(dockerSource))
			Expect(err).To(BeNil())

			someFile := "Hello from Docker!"
			someFileHeader := &tar.Header{
				Name: "some_file.txt",
				Size: int64(len(someFile)),
			}
			err = tarWriter.WriteHeader(someFileHeader)
			Expect(err).To(BeNil())
			_, err = tarWriter.Write([]byte(someFile))
			Expect(err).To(BeNil())

			err = tarWriter.Close()
			Expect(err).To(BeNil())
			err = gzipWriter.Close()
			Expect(err).To(BeNil())

			out := bytes.NewBuffer(nil)
			opts := provider.BuildImageOptions{
				Name:          "myimage",
				InputStream:   buf,
				OutputStream:  out,
				RawJSONStream: false,
			}
			err = prov.BuildImage(opts)
			defer exec.Command("docker", "rmi", "myimage:latest").Run()
			Expect(err).To(BeNil())

			Expect(out.String()).To(ContainSubstring("Successfully built"))

			matchingImage := getMatchingImage(prov)
			Expect(matchingImage).NotTo(BeNil())
			itCanRunBuiltImage(matchingImage.ID)
		})
	})

	Context("when a Dockerfile is provided as remote", func() {
		It("creates an Image successfully and correctly stores the additional data", func() {
			server := ghttp.NewServer()
			remoteURL := server.URL()
			defer server.Close()

			dockerSource := `
				FROM busybox:latest
				MAINTAINER Nitrous.IO <hello@nitrous.io>
				CMD /bin/sh -c 'printf "Hello from Docker!"'
			`

			server.AppendHandlers(
				ghttp.CombineHandlers(ghttp.RespondWith(http.StatusOK, dockerSource)),
			)

			out := bytes.NewBuffer(nil)
			opts := provider.BuildImageOptions{
				Name:          "myimage",
				InputStream:   nil,
				OutputStream:  out,
				RawJSONStream: false,
				Remote:        remoteURL,
			}
			err := prov.BuildImage(opts)
			defer exec.Command("docker", "rmi", "myimage:latest").Run()
			Expect(err).To(BeNil())

			Expect(out.String()).To(ContainSubstring("Successfully built"))

			matchingImage := getMatchingImage(prov)
			Expect(matchingImage).NotTo(BeNil())
			itCanRunBuiltImage(matchingImage.ID)
		})
	})

	Context("when a git repository is provided as remote", func() {
		It("creates an Image successfully and correctly stores the additional data", func() {
			remoteURL := "git://github.com/nitrous-io/docker-hello.git"
			out := bytes.NewBuffer(nil)
			opts := provider.BuildImageOptions{
				Name:          "myimage",
				InputStream:   nil,
				OutputStream:  out,
				RawJSONStream: false,
				Remote:        remoteURL,
			}
			err := prov.BuildImage(opts)
			defer exec.Command("docker", "rmi", "myimage:latest").Run()
			Expect(err).To(BeNil())

			Expect(out.String()).To(ContainSubstring("Successfully built"))

			matchingImage := getMatchingImage(prov)
			Expect(matchingImage).NotTo(BeNil())
			itCanRunBuiltImage(matchingImage.ID)
		})
	})
})
