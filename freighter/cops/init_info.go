package cops

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/pkg/reexec"
)

const infoCommandName = "cops-fileinfo"

func init() {
	reexec.Register(infoCommandName, infoInit)
}

// infoInit runs inside the container mnt namespace, and return fileinfo of a specific path
func infoInit() {
	runtime.LockOSThread()
	flag.Parse()
	id := flag.Arg(0)
	localPath := flag.Arg(1)

	// chroot the current process into path
	if err := syscall.Chdir("/"); err != nil {
		logrus.Fatalf("infoInit failed to change dir to / : %v", err)
	}

	stat, err := os.Stat(localPath)
	if err != nil {
		logrus.Fatalf("infoInit failed to stat %s: %v", localPath, err)
	}

	info := &FileInfo{
		CID:   id,
		Name:  stat.Name(),
		IsDir: stat.IsDir(),
	}
	if err := json.NewEncoder(os.Stdout).Encode(info); err != nil {
		logrus.Fatalf("infoInit failed to encode: %v", err)
	}
	os.Exit(0)
}

// Get FileInfo of a file inside the container
func GetFileInfo(id string, localPath string) (*FileInfo, error) {
	initPID, err := getContainerPID(id)
	if err != nil {
		return nil, fmt.Errorf("failed to get container %s pid: %v", id, err)
	}
	cmd := newCommand(infoCommandName, initPID, localPath)
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
