package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

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

	It("runs the command specified by the NITROUS_CMD env var while preserving Docker env vars", func() {
		config := `{
			"Env": [
				"NITROUS_CMD=echo $TEST_ENV > /test-output",
				"TEST_ENV=it works"
			],
			"Cmd": ["top"]
		}`
		_, info, cleanupFn, err := startContainer(config)
		defer cleanupFn()
		Expect(err).To(BeNil(), "failed to start container")

		// Give the docker exec command some time to run.
		time.Sleep(50 * time.Millisecond)

		out, _ := exec.Command("docker", "exec", info.ID, "cat", "/test-output").CombinedOutput()
		Expect(string(out)).To(Equal("it works\n"))
	})
})
