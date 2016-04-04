package integration

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CreateStart", func() {
	It("creates the container with the specified config", func() {
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
		_, info, cleanupFn, err := startContainerWithData("busybox", datadir, config)
		defer cleanupFn()
		Expect(err).To(BeNil())

		assertEnv(info.ID, []string{"LANG=C"}, nil)
		assertPort(info.ID, []interface{}{
			fmt.Sprintf("3000/tcp -> %s:65000", info.Gateway),
			"9999/tcp -> 0.0.0.0:9999",
		})
		assertVolumes(info.ID, map[string]string{
			"/etc/dotfiles":            filepath.Join(datadir, "volumes", "dotfiles"),
			"/volumes/backups":         filepath.Join(datadir, "volumes", "backups"),
			"/nitrous/ssh":             filepath.Join(datadir, "containers", info.Name, "ssh"),
			"/.nitrous-container-slug": filepath.Join(datadir, "containers", info.Name, ".nitrous-container-slug"),
		}, nil)
	})

	It("creates host directory for built-in volumes implicitly", func() {
		var err error
		datadir := os.Getenv("FREIGHTERD_BASE")
		if datadir == "" {
			datadir, err = ioutil.TempDir("", "freighter-integration")
			Expect(err).To(BeNil())
		}
		config := `{
			"Mounts": ["dotfiles:/etc/dotfiles","backups"]
		}`
		_, info, cleanupFn, err := startContainerWithData("busybox-volumes", datadir, config)
		defer cleanupFn()
		Expect(err).To(BeNil())

		Expect(info.Volumes).To(HaveLen(4), "unexpected returned volumes")
		Expect(info.Volumes["/var/lib/docker"]).To(HavePrefix(info.Name))
		Expect(info.Volumes["/var/lib/extra"]).To(HavePrefix(info.Name))
		Expect(info.Volumes["/var/lib/docker"]).ToNot(Equal(info.Volumes["/var/lib/extra"]))
		Expect(info.Volumes).To(HaveKeyWithValue("/etc/dotfiles", "dotfiles"))
		Expect(info.Volumes).To(HaveKeyWithValue("/volumes/backups", "backups"))

		assertVolumes(info.ID, map[string]string{
			"/etc/dotfiles":    filepath.Join(datadir, "volumes", "dotfiles"),
			"/volumes/backups": filepath.Join(datadir, "volumes", "backups"),
			"/var/lib/docker":  filepath.Join(datadir, "volumes", info.Volumes["/var/lib/docker"]),
			"/var/lib/extra":   filepath.Join(datadir, "volumes", info.Volumes["/var/lib/extra"]),
		}, nil)
	})

	It("returns an ErrImageNotFound when creating from an image that does not exist", func() {
		datadir, err := test.TempDir()
		Expect(err).To(BeNil())
		defer os.RemoveAll(datadir)
		prov, _ := docker.NewDockerProvider(datadir)

		_, err = prov.CreateStart(test.RandomName("freighterd-test"), "non-existent-image", "secret", "")
		Expect(err).To(MatchError(provider.ErrImageNotFound))
	})

	It("creates and starts if a CMD is specified in the container config", func() {
		datadir, err := test.TempDir()
		Expect(err).To(BeNil())
		defer os.RemoveAll(datadir)
		prov, _ := docker.NewDockerProvider(datadir)

		name := test.RandomName("freighterd-test")
		configJSON := `{ "Cmd": ["yes"] }`
		cnr, err := prov.CreateStart(name, "busybox:latest", "secret", configJSON)
		defer test.CleanUpContainer(name)

		Expect(err).To(BeNil())
		Expect(cnr.Cmd).To(Equal([]string{"yes"}))
	})

	It("writes configured scripts into a .nitrous-scripts.json file", func() {
		// Build a Docker image with an existing .nitrous-scripts.json, so that
		// we can test that its contents are overridden.
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

		datadir, err := test.TempDir()
		Expect(err).To(BeNil())
		defer os.RemoveAll(datadir)
		prov, err := docker.NewDockerProvider(datadir)
		Expect(err).To(BeNil())

		// Build the image.
		err = prov.BuildImage(provider.BuildImageOptions{
			Name:         "create-start-test",
			InputStream:  buf,
			OutputStream: ioutil.Discard,
		})
		defer exec.Command("docker", "rmi", "create-start-test:latest").Run()
		Expect(err).To(BeNil())

		// Specify some scripts to test that .nitrous-scripts.json gets
		// overriden.
		configJSON := `{
			"Scripts": {
				"start": "rails s -b 0.0.0.0",
				"List files": "ls -alh"
			}
		}`
		containerName := test.RandomName("freighterd-test")
		info, err := prov.CreateStart(containerName, "create-start-test:latest", "secret", configJSON)
		defer test.CleanUpContainer(containerName)
		Expect(err).To(BeNil())

		tmpOutputDir, err := test.TempDir()
		Expect(err).To(BeNil())
		defer os.RemoveAll(tmpOutputDir)

		out, err := exec.Command("docker", "cp",
			fmt.Sprintf("%s:/home/nitrous/.nitrous-scripts.json", info.ID),
			tmpOutputDir).CombinedOutput()
		Expect(err).To(BeNil(), "failed to copy .nitrous-scripts.json from container, output: %s, err: %v", out, err)

		copiedFile, err := os.Open(filepath.Join(tmpOutputDir, ".nitrous-scripts.json"))
		Expect(err).To(BeNil())
		defer copiedFile.Close()

		scripts := map[string]string{}
		err = json.NewDecoder(copiedFile).Decode(&scripts)
		Expect(err).To(BeNil())

		Expect(scripts).To(HaveLen(3))
		Expect(scripts["start"]).To(Equal("rails s -b 0.0.0.0"))
		Expect(scripts["run-tests"]).To(Equal("rake test"))
		Expect(scripts["List files"]).To(Equal("ls -alh"))

		// Check file owner and group.
		out, err = exec.Command("docker", "exec", info.ID,
			"/bin/stat", "--format", "'%U %G'", "/home/nitrous/.nitrous-scripts.json").CombinedOutput()
		Expect(err).To(BeNil(), "failed to copy stat .nitrous-scripts.json, output: %s, err: %v", out, err)
		Expect(string(out)).To(Equal("nitrous nitrous"))

		// Check file permissions.
		out, err = exec.Command("docker", "exec", info.ID,
			"/bin/stat", "--format", "%A", "/home/nitrous/.nitrous-scripts.json").CombinedOutput()
		Expect(err).To(BeNil(), "failed to copy stat .nitrous-scripts.json, output: %s, err: %v", out, err)
		Expect(string(out)).To(Equal("-rw-r--r--"))
	})

	It("does not change .nitrous-scripts.json file if there are no configured scripts", func() {
		// Build a Docker image with an existing .nitrous-scripts.json, so that
		// we can test that its contents are preserved.
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

		datadir, err := test.TempDir()
		Expect(err).To(BeNil())
		defer os.RemoveAll(datadir)
		prov, err := docker.NewDockerProvider(datadir)
		Expect(err).To(BeNil())

		// Build the image.
		err = prov.BuildImage(provider.BuildImageOptions{
			Name:         "create-start-test",
			InputStream:  buf,
			OutputStream: ioutil.Discard,
		})
		defer exec.Command("docker", "rmi", "create-start-test:latest").Run()
		Expect(err).To(BeNil())

		// Create a container with no scripts specified - .nitrous-scripts.json
		// should be unchanged.
		configJSON := `{}`
		containerName := test.RandomName("freighterd-test")
		info, err := prov.CreateStart(containerName, "create-start-test:latest", "secret", configJSON)
		defer test.CleanUpContainer(containerName)
		Expect(err).To(BeNil())

		tmpOutputDir, err := test.TempDir()
		Expect(err).To(BeNil())
		defer os.RemoveAll(tmpOutputDir)

		out, err := exec.Command("docker", "cp",
			fmt.Sprintf("%s:/home/nitrous/.nitrous-scripts.json", info.ID),
			tmpOutputDir).CombinedOutput()
		Expect(err).To(BeNil(), "failed to copy .nitrous-scripts.json from container, output: %s, err: %v", out, err)

		copiedFile, err := os.Open(filepath.Join(tmpOutputDir, ".nitrous-scripts.json"))
		Expect(err).To(BeNil())
		defer copiedFile.Close()

		scripts := map[string]string{}
		err = json.NewDecoder(copiedFile).Decode(&scripts)
		Expect(err).To(BeNil())

		Expect(scripts).To(HaveLen(2))
		Expect(scripts["start"]).To(Equal("rails s"))
		Expect(scripts["run-tests"]).To(Equal("rake test"))
	})

	Context("when container is created but cannot be started", func() {
		It("creates a stopped container and returns the container info and a StartFailedError", func() {
			datadir, err := test.TempDir()
			Expect(err).To(BeNil())
			defer os.RemoveAll(datadir)
			prov, _ := docker.NewDockerProvider(datadir)

			name := test.RandomName("freighterd-test")
			configJSON := `{ "Cmd": ["non-existent-cmd"] }`
			info, err := prov.CreateStart(name, "busybox:latest", "secret", configJSON)
			defer test.CleanUpContainer(name)

			_, ok := err.(*provider.StartFailedError)
			Expect(ok).To(BeTrue())

			Expect(info.Name).To(Equal(name))
			Expect(info.Image).To(Equal("busybox:latest"))
			Expect(info.State).To(Equal(provider.ContainerStateStopped))
			Expect(info.Host).To(Equal(""))
			Expect(info.Gateway).To(Equal(""))
		})
	})

	// This is part of creating a container. Since it is hard to create an image with volume config,
	// Separate it into different functions and test it individually.
	Describe("FixImage", func() {
		It("removes .nitrous-container-slug file", func() {
			// Create a empty file in the image directly
			datadir, err := test.TempDir()
			Expect(err).To(BeNil())
			defer os.RemoveAll(datadir)
			prov, _ := docker.NewDockerProvider(datadir)

			output, err := exec.Command("docker", "inspect", "-f", "{{.Id}}", "busybox:latest").Output()
			Expect(err).To(BeNil())

			imageID := strings.TrimSpace(string(output))
			invalidFilePath := fmt.Sprintf("/var/lib/docker/aufs/diff/%s/.nitrous-container-slug", imageID)

			f, err := os.Create(invalidFilePath)
			Expect(err).To(BeNil())
			f.Close()

			defer os.Remove(invalidFilePath)

			err = prov.FixImage("busybox:latest")
			Expect(err).To(BeNil())

			_, err = os.Stat(invalidFilePath)
			Expect(err).NotTo(BeNil())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("removes .nitrous-container-slug file from parent if current image does not have it", func() {
			// Create a empty file in the image directly
			datadir, err := test.TempDir()
			Expect(err).To(BeNil())
			defer os.RemoveAll(datadir)
			prov, _ := docker.NewDockerProvider(datadir)

			output, err := exec.Command("docker", "inspect", "-f", "{{.Parent}}", "busybox:latest").Output()
			Expect(err).To(BeNil())

			imageID := strings.TrimSpace(string(output))
			invalidFilePath := fmt.Sprintf("/var/lib/docker/aufs/diff/%s/.nitrous-container-slug", imageID)

			f, err := os.Create(invalidFilePath)
			Expect(err).To(BeNil())
			f.Close()

			defer os.Remove(invalidFilePath)

			err = prov.FixImage("busybox:latest")
			Expect(err).To(BeNil())

			_, err = os.Stat(invalidFilePath)
			Expect(err).NotTo(BeNil())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("return error if .nitrous-container-slug does not exist", func() {
			// Create a empty file in the image directly
			datadir, err := test.TempDir()
			Expect(err).To(BeNil())
			defer os.RemoveAll(datadir)
			prov, _ := docker.NewDockerProvider(datadir)

			err = prov.FixImage("busybox:latest")
			Expect(err).NotTo(BeNil())
		})
	})

	Describe("RemoveContainerWithName", func() {
		It("removes a container with name", func() {
			datadir, err := test.TempDir()
			Expect(err).To(BeNil())
			defer os.RemoveAll(datadir)
			prov, _ := docker.NewDockerProvider(datadir)

			name := test.RandomName("freighterd-test")
			configJSON := `{ "Cmd": ["yes"] }`
			cnr, err := prov.CreateStart(name, "busybox:latest", "secret", configJSON)
			Expect(err).To(BeNil())
			defer test.CleanUpContainer(name)

			// Stop container
			cnr, err = prov.Stop(cnr.ID)
			Expect(err).To(BeNil())

			// To check if the container exists
			err = exec.Command("docker", "inspect", cnr.ID).Run()
			Expect(err).To(BeNil())

			err = prov.RemoveContainerWithName(name)
			Expect(err).To(BeNil())

			// To check if the container exists
			err = exec.Command("docker", "inspect", cnr.ID).Run()
			Expect(err).NotTo(BeNil())
		})
	})

})
