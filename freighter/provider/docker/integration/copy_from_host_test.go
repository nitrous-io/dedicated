package integration

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CopyFromHost", func() {
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

	It("copies input into the container's location", func() {
		prov, dataPath, err := createNewProvider()
		Expect(err).To(BeNil(), "failed to initialize docker provider")
		defer os.RemoveAll(dataPath)

		name := test.RandomName("freightertest")
		configJSON := `{
			"Cmd": ["/bin/sh", "-c", "while true; do sleep 1; echo hello; done"]
		}`
		info, err := prov.CreateStart(name, "busybox:latest", "secret", configJSON)
		defer test.CleanUpContainer(name)
		Expect(err).To(BeNil())

		input := map[string]io.Reader{
			"hello": bytes.NewBufferString("hello world"),
		}
		err = prov.CopyFromHost(info.ID, "/", input)
		Expect(err).To(BeNil(), "unexpected errror when copy from host")

		// read the file directly from /var/lib/docker
		fp := filepath.Join("/var/lib/docker/aufs/mnt", info.ID, "hello")
		output, err := ioutil.ReadFile(fp)
		Expect(err).To(BeNil(), "file wasnt created")
		Expect(string(output)).To(Equal("hello world"), "invalid content of file")
	})
})
