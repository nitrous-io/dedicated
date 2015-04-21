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

const readFileCommandName = "cops-readfile"

func init() {
	reexec.Register(readFileCommandName, readFileInit)
}

// infoInit runs inside the container mnt namespace, and write contents
// of the local file to stdout
func readFileInit() {
	runtime.LockOSThread()
	flag.Parse()
	// id  is arg 0
	localPath := flag.Arg(1)

	if err := syscall.Chdir("/"); err != nil {
		log.Fatalf("readFileInit failed to change dir to / : %v", err)
	}

	f, err := os.Open(localPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	io.Copy(os.Stdout, f)
	os.Exit(0)
}

// Write contents of a path inside container to the writer.
// This should be used only for file. For directory, it's best to use docker's
// built-in copy
func ReadFromContainer(id string, localPath string, w io.Writer) error {
	initPID, err := getContainerPID(id)
	if err != nil {
		return fmt.Errorf("failed to get container %s pid: %v", id, err)
	}
	// create the command, pass the child as fd3 of the command
	cmd := reexec.Command(readFileCommandName, strconv.Itoa(initPID), localPath)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
