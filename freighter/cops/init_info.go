package cops

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/nitrous-io/dedicated/pkg/reexec"
)

const infoCommandName = "cops-fileinfo"

func init() {
	reexec.Register(infoCommandName, infoInit)
}

// infoInit runs inside the container chroot, and return fileinfo of a specific path
func infoInit() {
	runtime.LockOSThread()
	flag.Parse()
	id := flag.Arg(0)
	path := flag.Arg(1)
	localPath := flag.Arg(2)

	// chroot the current process into path
	if err := syscall.Chroot(path); err != nil {
		log.Fatalf("infoInit failed to chroot to %s : %v", path, err)
	}
	if err := syscall.Chdir("/"); err != nil {
		log.Fatalf("infoInit failed to change dir to / : %v", err)
	}

	stat, err := os.Stat(localPath)
	if err != nil {
		log.Fatalf("infoInit failed to stat %s: %v", localPath, err)
	}

	info := &FileInfo{
		CID:   id,
		Name:  stat.Name(),
		IsDir: stat.IsDir(),
	}
	if err := json.NewEncoder(os.Stdout).Encode(info); err != nil {
		log.Fatal("infoInit failed to encode: %v", err)
	}
	os.Exit(0)
}

// Get FileInfo of a file inside the container
func GetFileInfo(id string, localPath string) (*FileInfo, error) {
	root := filepath.Join(RootPrefix, id)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to find rootfs for %s, %s", id, err)
	}
	cmd := reexec.Command(infoCommandName, id, root, localPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	info := &FileInfo{}
	if err := json.Unmarshal(out, info); err != nil {
		return nil, err
	}
	return info, nil
}
