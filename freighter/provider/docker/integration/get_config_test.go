package integration

import (
	"archive/tar"
	"bytes"
	"io/ioutil"
	"os/exec"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/test"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetConfig", func() {
	It("retrieves port bindings for running and stopped containers", func() {
		datadir, err := ioutil.TempDir("", "freightertest")
		Expect(err).To(BeNil())

		prov, err := docker.NewDockerProvider(datadir)
		Expect(err).To(BeNil())

		name := test.RandomName("freightertest")
		configJSON := `{
			"PortBindings": ["private:1234:2345/udp", "public:2345:3456/tcp", "public::3456", "private::8080"]
		}`
		info, err := prov.CreateStart(name, "busybox:latest", "secret", configJSON)
		defer test.CleanUpContainer(name)
		Expect(err).To(BeNil())

		config, err := prov.GetConfig(info.ID)
		Expect(err).To(BeNil())

		Expect(config.PortBindings).To(ContainElement("private:1234:2345/udp"))
		Expect(config.PortBindings).To(ContainElement("public:2345:3456/tcp"))
		Expect(config.PortBindings).To(ContainElement(MatchRegexp(`public:\d+:3456`)))
		Expect(config.PortBindings).To(ContainElement(MatchRegexp(`private:\d+:8080`)))

		Expect(config.ConfigPortBindings).To(ConsistOf([]string{
			"private:1234:2345/udp", "public:2345:3456/tcp", "public::3456/tcp", "private::8080/tcp",
		}))

		info, err = prov.Stop(name)
		Expect(err).To(BeNil())

		config, err = prov.GetConfig(info.ID)
		Expect(err).To(BeNil())

		Expect(config.PortBindings).To(ConsistOf([]string{
			"private:1234:2345/udp", "public:2345:3456/tcp", "public::3456/tcp", "private::8080/tcp",
		}))

		Expect(config.ConfigPortBindings).To(ConsistOf([]string{
			"private:1234:2345/udp", "public:2345:3456/tcp", "public::3456/tcp", "private::8080/tcp",
		}))
	})

	It("returns scripts from .nitrous-scripts.json in the container's filesystem", func() {
		// Build a Docker image with a .nitrous-scripts.json so that we can test
		// that its contents are returned.
		dockerfile := `
			FROM busybox:latest
			ENV NITROUS_USERNAME nitrous
			RUN adduser nitrous -D
			COPY .nitrous-scripts.json /home/nitrous/.nitrous-scripts.json
			ENTRYPOINT top`

		buf := new(bytes.Buffer)
		tw := tar.NewWriter(buf)

		err := tw.WriteHeader(&tar.Header{
			Name: "Dockerfile",
			Size: int64(len(dockerfile)),
		})
		Expect(err).To(BeNil())
		_, err = tw.Write([]byte(dockerfile))
		Expect(err).To(BeNil())

		containerScripts := `{ "start": "rails s", "run-tests": "rake test" }`
		err = tw.WriteHeader(&tar.Header{
			Name: ".nitrous-scripts.json",
			Size: int64(len(containerScripts)),
		})
		Expect(err).To(BeNil())
		_, err = tw.Write([]byte(containerScripts))
		Expect(err).To(BeNil())

		err = tw.Close()
		Expect(err).To(BeNil())

		datadir, err := ioutil.TempDir("", "freightertest")
		Expect(err).To(BeNil())
		prov, err := docker.NewDockerProvider(datadir)
		Expect(err).To(BeNil())

		// Build the image.
		err = prov.BuildImage(provider.BuildImageOptions{
			Name:         "getconfig-test",
			InputStream:  buf,
			OutputStream: ioutil.Discard,
		})
		defer exec.Command("docker", "rmi", "getconfig-test:latest").Run()
		Expect(err).To(BeNil())

		containerName := test.RandomName("freightertest")
		info, err := prov.CreateStart(containerName, "getconfig-test:latest", "secret", "")
		defer test.CleanUpContainer(containerName)
		Expect(err).To(BeNil())

		config, err := prov.GetConfig(info.ID)
		Expect(err).To(BeNil())

		Expect(config.Scripts).To(HaveLen(2))
		Expect(config.Scripts["start"]).To(Equal("rails s"))
		Expect(config.Scripts["run-tests"]).To(Equal("rake test"))

		// Check that no scripts are returned if container is not running.
		info, err = prov.Stop(containerName)
		Expect(err).To(BeNil())

		config, err = prov.GetConfig(info.ID)
		Expect(err).To(BeNil())
		Expect(config.Scripts).To(BeNil())
	})

	It("returns no scripts if the container does not contain a .nitrous-scripts.json", func() {
		datadir, err := ioutil.TempDir("", "freightertest")
		Expect(err).To(BeNil())
		prov, err := docker.NewDockerProvider(datadir)
		Expect(err).To(BeNil())

		containerName := test.RandomName("freightertest")
		info, err := prov.CreateStart(containerName, "busybox:latest", "secret", "")
		defer test.CleanUpContainer(containerName)
		Expect(err).To(BeNil())

		config, err := prov.GetConfig(info.ID)
		Expect(err).To(BeNil())

		Expect(config.Scripts).To(HaveLen(0))
	})
})
