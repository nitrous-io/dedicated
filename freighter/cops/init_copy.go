package cops

import (
	"encoding/json"
	"flag"
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
	// require init pid to be argv 0th
	localPath := flag.Arg(1)

	if err := syscall.Chdir("/"); err != nil {
		log.Fatalf("copyInit failed to change dir to / : %v", err)
	}

	// this is used to communicate with the parent process
	infoR := os.NewFile(3, "info")
	defer infoR.Close()
	info := &CopsInfo{}
	if err := json.NewDecoder(infoR).Decode(info); err != nil {
		log.Fatalf("%s cmd: failed to decode info %v", copyCommandName, err)
	}
	uid, gid := info.owner()

	// create the file if not exists and truncate it if exists
	f, err := os.Create(localPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	// rename the file to correct uid/gid
	if err := f.Chown(uid, gid); err != nil {
		log.Fatalf("%s cmd: %v", copyCommandName, err)
	}
	if _, err := io.Copy(f, os.Stdin); err != nil {
		log.Fatalf("%s cmd: %v", copyCommandName, err)
	}
	os.Exit(0)
}

// Copy contents of the reader into the path inside container
func CopyToContainer(id string, env []string, localPath string, r io.Reader) error {
	info, err := getCopsInfo(id, env)
	if err != nil {
		return err
	}
	// create a pipe to pass info over
	infoR, infoW, err := os.Pipe()
	if err != nil {
		return err
	}
	defer infoW.Close()

	// create the command, pass the child as fd3 of the command
	cmd := reexec.Command(copyCommandName, strconv.Itoa(info.PID), localPath)
	cmd.Stdin = r
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{infoR}
	if err := cmd.Start(); err != nil {
		infoR.Close()
		return err
	}
	infoR.Close()
	if err := json.NewEncoder(infoW).Encode(info); err != nil {
		log.Println("%s cmd: failed to write info %v", copyCommandName, err)
		cmd.Process.Kill()
		return err
	}
	return cmd.Wait()
}
