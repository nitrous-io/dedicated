package integration

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CreateStart", func() {
	It("creates a running container", func() {
		// start the container with busybox image
		datadir, _ := test.TempDir()
		_, info, cleanupFn, err := startContainerWithData("nitrous/busybox:latest", datadir, "")
		defer cleanupFn()
		Expect(err).To(BeNil())

		client, _ := dockerclient.NewClient("unix:///var/run/docker.sock")
		container, _ := client.InspectContainer(info.ID)

		// assert returned container's info
		Expect(info.ID).To(Equal(container.ID), "unexpected container's id")
		Expect("/"+info.Name).To(Equal(container.Name), "unexpected container's name")
		Expect(info.Image).To(Equal("nitrous/busybox:latest"), "unexpected base image")
		Expect(info.ImageID).To(Equal(container.Image), "unexpected image id")
		Expect(info.Host).To(Equal(container.NetworkSettings.IPAddress), "unexpected ip")
		Expect(info.Gateway).To(Equal(container.NetworkSettings.Gateway), "unexpected gateway")
		Expect(info.State).To(Equal(provider.ContainerStateRunning), "expected running container")
		Expect(info.Env).To(ContainElement(fmt.Sprintf("__HOST=%s", container.NetworkSettings.Gateway)), "unexpected __HOST")
		Expect(info.Cmd).To(Equal(container.Config.Cmd), "unexpected cmd")
		Expect(info.Entrypoint).To(Equal(container.Config.Entrypoint), "unexpected entry point")
		Expect(info.ExitCode).To(Equal(container.State.ExitCode), "unexpected exit code")
		Expect(info.StartedAt).To(Equal(container.State.StartedAt), "unexpected started at")
		Expect(info.FinishedAt).To(Equal(container.State.FinishedAt), "unexpected finished at")
		Expect(info.Ports).To(HaveKey("22"), "failed to expose 22 for nitrous/busybox")
		Expect(container.NetworkSettings.Ports).To(HaveKey(dockerclient.Port("22/tcp")), "failed to expose 22/tcp for nitrous/busybox")

		// assert settings directory to be created
		settingsDir := path.Join(datadir, "containers", container.Name)
		_, err = os.Stat(settingsDir)
		Expect(err).To(BeNil()) // container settings dir exists

		// authorized_keys file exists
		authKeysFile := path.Join(settingsDir, "ssh", "authorized_keys")
		_, err = os.Stat(authKeysFile)
		Expect(err).To(BeNil())

		// secret file exists
		sharedSecretFile := path.Join(settingsDir, "secret")
		fi, _ := os.Stat(sharedSecretFile)
		Expect(fi.Mode()).To(Equal(os.FileMode(0600))) // shared secret file has perm 0600
		s, _ := ioutil.ReadFile(sharedSecretFile)
		Expect(string(s)).To(Equal("secret"))
	})

	It("creates the container with specified options", func() {
		var err error
		datadir := os.Getenv("FREIGHTERD_BASE")
		if datadir == "" {
			datadir, err = test.TempDir()
			Expect(err).To(BeNil())
		}

		config := `{
			"Env": ["LANG=C"],
			"Mounts": ["dotfiles:/etc/dotfiles","backups"],
			"PortBindings": ["private:65000:3000", "public:9999:9999"]
		}`
		_, info, cleanupFn, err := startContainerWithData("busybox:latest", datadir, config)
		defer cleanupFn()
		Expect(err).To(BeNil())

		assertEnv(info.ID, []string{"LANG=C"}, nil)
		assertPort(info.ID, []interface{}{
			fmt.Sprintf("3000/tcp -> %s:65000", info.Gateway),
			"9999/tcp -> 0.0.0.0:9999",
		})
		assertVolumes(info.ID, map[string]string{
			"/etc/dotfiles":    filepath.Join(datadir, "volumes", "dotfiles"),
			"/volumes/backups": filepath.Join(datadir, "volumes", "backups"),
			"/nitrous/ssh":     filepath.Join(datadir, "containers", info.Name, "ssh"),
		}, nil)
	})

	It("runs the command specified by the NITROUS_CMD env var while preserving Docker env vars", func() {
		config := `{
			"Env": [
				"NITROUS_CMD=echo $TEST_ENV > /test-output",
				"TEST_ENV=it works"
			],
			"Cmd": ["top"]
		}`
		_, info, cleanupFn, err := startContainer("busybox:latest", config)
		defer cleanupFn()
		Expect(err).To(BeNil(), "failed to start container")

		// Give the docker exec command some time to run.
		time.Sleep(50 * time.Millisecond)

		out, _ := exec.Command("docker", "exec", info.ID, "cat", "/test-output").CombinedOutput()
		Expect(string(out)).To(Equal("it works\n"))
	})
})
