package integration

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/nitrous-io/dedicated/freighter/provider"
	"github.com/nitrous-io/dedicated/freighter/provider/docker"
	"github.com/nitrous-io/dedicated/freighter/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CreateStart", func() {
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

	Context("when image has no CMD or ENTRYPOINT", func() {
		It("creates and starts if CMD is given to container config", func() {
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
})
