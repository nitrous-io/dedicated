package cops

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"syscall"

	"github.com/nitrous-io/dedicated/pkg/reexec"
)

const copyCommandName = "cops-copy"

func init() {
	reexec.Register(copyCommandName, copyInit)
}

// copyInit runs inside the container mnt namespace, and copy its stdin
// into the specific path
func copyInit() {
	runtime.LockOSThread()
	flag.Parse()
	// id is arg 0
	localPath := flag.Arg(1)

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
	initPID, err := GetContainerPID(id)
	if err != nil {
		return fmt.Errorf("failed to get container %s pid: %v", id, err)
	}
	log.Println("got pid", initPID)
	// create the command, pass the child as fd3 of the command
	cmd := reexec.Command(copyCommandName, strconv.Itoa(initPID), localPath)
	cmd.Stdin = r
	// pipe the output/errors to os stdout/stderr
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
