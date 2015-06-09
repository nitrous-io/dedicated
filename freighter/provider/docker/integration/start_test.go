package integration

import (
	"fmt"
	"os"
	"path/filepath"

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
