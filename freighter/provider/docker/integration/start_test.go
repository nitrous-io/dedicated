package integration

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Start", func() {
	It("keeps PortBinding and Binds if we dont specify different options", func() {
		var err error
		datadir := os.Getenv("FREIGHTERD_BASE")
		if datadir == "" {
			datadir, err = test.TempDir()
			Expect(err).To(BeNil())
		}

		config := `{
			"Mounts": ["dotfiles:/etc/dotfiles","backups"],
			"PortBindings": ["private:65000:3000", "public:9999:9999"]
		}`
		prov, info, cleanupFn, err := startContainerWithData("busybox", datadir, config)
		defer cleanupFn()
		Expect(err).To(BeNil())

		_, err = prov.Stop(info.ID)
		Expect(err).To(BeNil(), "failed to stop %s", info.ID)
		_, err = prov.Start(info.ID, "")
		Expect(err).To(BeNil(), "failed to start %s", info.ID)

		assertPort(info.ID, []interface{}{
			fmt.Sprintf("3000/tcp -> %s:65000", info.Gateway),
			"9999/tcp -> 0.0.0.0:9999",
		})
		assertVolumes(info.ID, map[string]string{
			"/etc/dotfiles":    filepath.Join(datadir, "volumes", "dotfiles"),
			"/volumes/backups": filepath.Join(datadir, "volumes", "backups"),
			"/nitrous/ssh":     filepath.Join(datadir, "containers", info.Name, "ssh"),
		}, nil)

		extraHosts, err := inspectField(info.ID, "HostConfig.ExtraHosts")
		Expect(err).To(BeNil(), "failed to get extra host")
		Expect(extraHosts).To(ContainSubstring(fmt.Sprintf("host:%s", info.Gateway)), "invalid extra host")
	})

	It("sets existing ports' bindings if options are given", func() {
		var err error
		datadir := os.Getenv("FREIGHTERD_BASE")
		if datadir == "" {
			datadir, err = test.TempDir()
			Expect(err).To(BeNil())
		}

		config := `{
			"Mounts": ["dotfiles:/etc/dotfiles","backups"],
			"PortBindings": ["private:65000:3000", "public:9999:9999"]
		}`
		prov, info, cleanupFn, err := startContainerWithData("busybox", datadir, config)
		defer cleanupFn()
		Expect(err).To(BeNil())

		_, err = prov.Stop(info.ID)
		Expect(err).To(BeNil(), "failed to stop %s", info.ID)

		config = `{
			"Mounts": ["dotfiles:/etc/dotfiles","restores"],
			"PortBindings": ["private:6000:3000", "public:65000:9999"]
		}`
		_, err = prov.Start(info.ID, config)
		Expect(err).To(BeNil(), "failed to start %s", info.ID)

		assertPort(info.ID, []interface{}{
			fmt.Sprintf("3000/tcp -> %s:6000", info.Gateway),
			"9999/tcp -> 0.0.0.0:65000",
		})
		assertVolumes(info.ID, map[string]string{
			"/etc/dotfiles":     filepath.Join(datadir, "volumes", "dotfiles"),
			"/volumes/restores": filepath.Join(datadir, "volumes", "restores"),
			"/nitrous/ssh":      filepath.Join(datadir, "containers", info.Name, "ssh"),
		}, nil)

		binds, err := inspectField(info.ID, "HostConfig.Binds")
		Expect(err).To(BeNil(), "failed to get binds")
		Expect(binds).NotTo(ContainSubstring("backups"), "invalid host binds")
	})

	It("runs the script labeled as 'start' in the container's .nitrous-scripts.json file", func() {
		// Build a Docker image with a NITROUS_USERNAME user.
		dockerfile := `
			FROM nitrousio/ubuntu-bare:latest
			ENV NITROUS_USERNAME nitrous
			RUN adduser nitrous -D || true
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

		err = tw.Close()
		Expect(err).To(BeNil())

		datadir, err := test.TempDir()
		Expect(err).To(BeNil())
		defer os.RemoveAll(datadir)
		prov, err := docker.NewDockerProvider(datadir)
		Expect(err).To(BeNil())

		// Build the image.
		err = prov.BuildImage(provider.BuildImageOptions{
			Name:         "start-test",
			InputStream:  buf,
			OutputStream: ioutil.Discard,
		})
		defer exec.Command("docker", "rmi", "start-test:latest").Run()
		Expect(err).To(BeNil())

		configJSON := ""
		containerName := test.RandomName("freighterd-test")
		info, err := prov.CreateStart(containerName, "start-test:latest", "secret", configJSON)
		defer test.CleanUpContainer(containerName)
		Expect(err).To(BeNil())

		// Create a .nitrous-scripts.json file - we will test that its 'start'
		// script gets run when the container is started.
		jsonSrc := `{ "start": "echo 'started' >> /home/nitrous/test-start.log " }`
		cmd := exec.Command("docker", "exec", info.ID,
			"/bin/sh", "-c",
			fmt.Sprintf("echo %q >> /home/nitrous/.nitrous-scripts.json", jsonSrc))
		out, err := cmd.CombinedOutput()
		Expect(err).To(BeNil(), "failed to run command %v in container, output: %s, err: %v", cmd, out, err)

		// Stop the container, then start it - this is what we are testing.
		_, err = prov.Stop(info.ID)
		Expect(err).To(BeNil())
		_, err = prov.Start(info.ID, "")
		Expect(err).To(BeNil())

		// cat the file that the 'start' script appends to to check it was
		// written to.
		cmd = exec.Command("docker", "exec", info.ID, "cat", "/home/nitrous/test-start.log")
		out, err = cmd.CombinedOutput()
		Expect(err).To(BeNil(), "failed to run command %v in container, output: %s, err: %v", cmd, out, err)

		Expect(string(out)).To(Equal("started\n"))
	})

	It("should expose the nitrous tools directory as a volume", func() {
		if os.Getenv("IN_DOCKER") == "true" {
			var err error

			err = os.Mkdir("/opt/nitrous", 0700)
			Expect(err).To(BeNil())

			defer os.Remove("/opt/nitrous")

			datadir := os.Getenv("FREIGHTERD_BASE")
			if datadir == "" {
				datadir, err = test.TempDir()
				Expect(err).To(BeNil())
			}

			config := `{
			  "Mounts": ["dotfiles:/etc/dotfiles","backups"],
			  "PortBindings": ["private:65000:3000", "public:9999:9999"]
			}`
			prov, info, cleanupFn, err := startContainerWithData("busybox", datadir, config)
			defer cleanupFn()
			Expect(err).To(BeNil())

			_, err = prov.Stop(info.ID)
			Expect(err).To(BeNil(), "failed to stop %s", info.ID)

			config = `{
			"Mounts": ["dotfiles:/etc/dotfiles","restores"],
			"PortBindings": ["private:6000:3000", "public:65000:9999"]
			}`
			_, err = prov.Start(info.ID, config)
			Expect(err).To(BeNil(), "failed to start %s", info.ID)

			assertVolumes(info.ID, map[string]string{
				"/etc/dotfiles":     filepath.Join(datadir, "volumes", "dotfiles"),
				"/volumes/restores": filepath.Join(datadir, "volumes", "restores"),
				"/nitrous/ssh":      filepath.Join(datadir, "containers", info.Name, "ssh"),
				"/.nitrous":         "/opt/nitrous",
			}, nil)
		} else {
			fmt.Fprintln(GinkgoWriter, "Test is disabled while outside Docker")
		}
	})
})
