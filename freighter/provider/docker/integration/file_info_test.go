package integration

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("FileInfo", func() {
	dockerfile := `
	FROM busybox:latest
	RUN mkdir /copydir && echo -n world > /copydir/hello
	RUN echo -n hello > /copyfile
	`

	createImageTarball := func(df string) (io.Reader, error) {
		buf := new(bytes.Buffer)
		tarWriter := tar.NewWriter(buf)
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: "Dockerfile", Size: int64(len(df))}); err != nil {
			return nil, err
		}
		if _, err := tarWriter.Write([]byte(df)); err != nil {
			return nil, err
		}
		if err := tarWriter.Close(); err != nil {
			return nil, err
		}
		return buf, nil
	}

	createNewProvider := func() (*docker.DockerProvider, string, error) {
		data, err := ioutil.TempDir("", "freightertest")
		if err != nil {
			return nil, "", err
		}
		prov, err := docker.NewDockerProvider(data)
		if err != nil {
			os.RemoveAll(data)
			return nil, "", err
		}
		return prov, data, nil
	}

	lookupFile := func(path string) (provider.FileInfo, error) {
		prov, dataPath, err := createNewProvider()
		Expect(err).To(BeNil(), "failed to initialize docker provider")
		defer os.RemoveAll(dataPath)

		input, err := createImageTarball(dockerfile)
		Expect(err).To(BeNil(), "failed to create dockerfile tarball")

		err = prov.BuildImage(provider.BuildImageOptions{
			Name:         "myimage",
			InputStream:  input,
			OutputStream: ioutil.Discard,
		})
		defer exec.Command("docker", "rmi", "myimage:latest").Run()
		Expect(err).To(BeNil())

		name := test.RandomName("freightertest")
		configJSON := `{
			"Cmd": ["/bin/sh", "-c", "while true; do sleep 1; echo hello; done"]
		}`
		info, err := prov.CreateStart(name, "myimage:latest", "secret", configJSON)
		defer func() {
			test.CleanUpContainer(name)
			exec.Command("docker", "rmi", "myimage:latest").Run()
		}()
		Expect(err).To(BeNil())

		return prov.FileInfo(info.ID, path)
	}

	It("returns an error if the file does not exist", func() {
		_, err := lookupFile("/foobar2")
		Expect(err).ToNot(BeNil())
	})

	It("returns info about the file", func() {
		inf, err := lookupFile("/copyfile")
		Expect(err).To(BeNil())
		Expect(inf.IsDir).To(BeFalse())
	})
})
