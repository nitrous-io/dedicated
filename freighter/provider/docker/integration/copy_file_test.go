package integration

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CopyFile", func() {
	dockerfile := `
	FROM busybox:latest
	RUN mkdir /copydir && echo -n world > /copydir/hello
	RUN echo -n hello > /copyfile
	`

	It("copies the path as tar archive when it's a directory", func() {
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

		r, w := io.Pipe()
		errCh := make(chan error, 1)
		go func() {
			defer w.Close()
			err := prov.CopyFile(info.ID, "/copydir", w)
			errCh <- err
		}()
		tr := tar.NewReader(r)
		files := []string{}
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			Expect(err).To(BeNil(), "unexpected error when reading tar output")
			files = append(files, hdr.Name)
			io.Copy(ioutil.Discard, tr)
		}
		Expect(files).To(ConsistOf("copydir/", "copydir/hello"), "invalid copied elements")
		err = <-errCh
		Expect(err).To(BeNil(), "unexpected errror when copy directory")
	})

	It("copies the path as file when it's a file", func() {
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

		buf := bytes.NewBuffer(nil)
		err = prov.CopyFile(info.ID, "/copyfile", buf)
		Expect(err).To(BeNil(), "failed to copy filed")
		Expect(buf.String()).To(Equal("hello"), "invalid copied file content")
	})
})
