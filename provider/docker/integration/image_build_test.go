package integration

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os/exec"

	"github.com/nitrous-io/freighter/provider"
	"github.com/nitrous-io/freighter/provider/docker"
	"github.com/nitrous-io/freighter/util"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("BuildImage", func() {
	createImageTarball := func() io.Reader {
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

		return buf
	}

	It("creates an Image successfully", func() {
		logger := &util.Logger{Level: util.LogLevelQuiet}
		data, err := ioutil.TempDir("", "freightertest")
		Expect(err).To(BeNil(), "failed to create temp dir")
		prov, err := docker.NewDockerProvider(logger, data)
		Expect(err).To(BeNil(), "failed to initialize docker provider")

		in := createImageTarball()
		out := bytes.NewBuffer(nil)
		err = prov.BuildImage("myimage", in, out, false)
		defer exec.Command("docker", "rmi", "myimage:latest").Run()
		Expect(err).To(BeNil())

		Expect(out.String()).To(ContainSubstring("Successfully built"))
	})

	It("correctly stores the additional data", func() {
		logger := &util.Logger{Level: util.LogLevelQuiet}
		data, err := ioutil.TempDir("", "freightertest")
		Expect(err).To(BeNil(), "failed to create temp dir")
		prov, err := docker.NewDockerProvider(logger, data)
		Expect(err).To(BeNil(), "failed to initialize docker provider")

		in := createImageTarball()
		out := bytes.NewBuffer(nil)
		err = prov.BuildImage("myimage", in, out, false)
		defer exec.Command("docker", "rmi", "myimage:latest").Run()
		Expect(err).To(BeNil())

		allImages, err := prov.ListImages()
		Expect(err).To(BeNil())

		var matchingImage *provider.ImageInfo
		for _, image := range allImages {
			if image.Name == "myimage" && image.Tag == "latest" {
				matchingImage = image
			}
		}

		Expect(matchingImage).ToNot(BeNil())

		res, err := exec.Command("docker", "run", "--rm=true", matchingImage.ID).CombinedOutput()
		Expect(err).To(BeNil(), "docker run failed with output: %s", string(res))
		Expect(string(res)).To(Equal("Hello from Docker!"))
	})
})
