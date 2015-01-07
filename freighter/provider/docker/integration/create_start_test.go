package integration

import (
	"fmt"
	"os"
	"path/filepath"

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
		_, info, cleanupFn, err := startContainerWithData(datadir, config)
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
})
