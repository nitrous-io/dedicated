package cops

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/nitrous-io/dedicated/pkg/reexec"
)

const copyCommandName = "cops-copy"

func init() {
	reexec.Register(copyCommandName, copyInit)
}

// copyInit runs inside the container, and copy its stdin
// into the specific path
func copyInit() {
	runtime.LockOSThread()
	flag.Parse()
	// id is the 0th argument
	path := flag.Arg(1)
	localPath := flag.Arg(2)

	// chroot the current process into path
	if err := syscall.Chroot(path); err != nil {
		log.Fatalf("copyInit failed to chroot to %s : %v", path, err)
	}
	if err := syscall.Chdir("/"); err != nil {
		log.Fatalf("copyInit failed to change dir to / : %v", err)
	}

	// create the file if not exists and truncate it if exists
	f, err := os.Create(localPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	if _, err := io.Copy(f, os.Stdin); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}

// Copy contents of the reader into the path inside container
func CopyToContainer(id string, localPath string, r io.Reader) error {
	root := filepath.Join(RootPrefix, id)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return fmt.Errorf("failed to find rootfs for %s, %s", id, err)
	}
	// create the command, pass the child as fd3 of the command
	cmd := reexec.Command(copyCommandName, id, root, localPath)
	cmd.Stdin = r
	// pipe the output/errors to os stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
