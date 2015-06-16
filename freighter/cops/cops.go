package cops

/*
__attribute__((constructor)) init() {
	cops_start();
}
*/
import "C"
import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/nitrous-io/dedicated/pkg/reexec"
)

type (
	// Message will be serialized over the transport. It is used to represent both
	// the operation and its result.
	Message struct {
		Namespace string        `json:"namespace"`
		Event     string        `json:"event"`
		CID       int64         `json:"cid"`
		Args      []interface{} `json:"args"`
		Error     string        `json:"error"`
	}

	FileInfo struct {
		CID   string
		Name  string
		IsDir bool
	}

	// CopsInfo has details about the container environment that cops
	// processes are being run in.
	CopsInfo struct {
		CID string   `json:"cid"` // id of the container
		PID int      `json:"pid"` // pid of init process
		UID int      `json:"uid"` // uid of init process
		GID int      `json:"gid"` // gid of init process
		Env []string `json:"env"` // env of init process
	}

	AsyncHandler interface {
		HandleAsync(msg *Message) chan *Message
	}
)

// newCommand returns an exec.Cmd in a form of reexec
// initPID is required so we can enter the init process's mount namespace
func newCommand(cmd string, initPID int, args ...string) *exec.Cmd {
	argv := []string{cmd, strconv.Itoa(initPID)}
	argv = append(argv, args...)
	return reexec.Command(argv...)
}

// getCopsInfo return data about the containerized environment that is sent over
// to cops process so it can setup properly
func getCopsInfo(containerID string, env []string) (*CopsInfo, error) {
	initPID, err := getContainerPID(containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container %s pid: %v", containerID, err)
	}
	uid, gid, err := getProcessOwner(initPID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container %s uid/gid: %v", containerID, err)
	}
	return &CopsInfo{
		CID: containerID,
		PID: initPID,
		UID: uid,
		GID: gid,
		Env: env,
	}, nil
}

// owner returns uid/gid of the desired user that cops should act as. This is
// helpful when we want to write a file and want to imitate as the current
// user.
func (c *CopsInfo) owner() (uid int, gid int) {
	var username string
	var ok bool
	for _, e := range c.Env {
		fields := strings.SplitN(e, "=", 2)
		k, v := fields[0], fields[1]
		if strings.ToLower(k) == "nitrous_username" {
			username = v
			ok = true
		}
	}
	if !ok {
		// return
		return c.UID, c.GID
	}
	// tries to locate current user uid/gid from local /etc/passwd
	uid, gid, err := parsePasswdFile("/etc/passwd", username)
	if err != nil {
		logrus.Errorf("failed to find user %s in container %s", username, c.CID)
		return c.UID, c.GID
	}
	return uid, gid
}

// parse a password file and returns the uid/gid asssociated with the username
func parsePasswdFile(path, username string) (uid int, gid int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	var found bool
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		// ignore if the line is malformed or it doesnt matched the username
		if len(fields) < 4 || fields[0] != username {
			continue
		}

		uid, err = strconv.Atoi(fields[2])
		if err != nil {
			break
		}
		gid, err = strconv.Atoi(fields[3])
		if err != nil {
			break
		}
		found = true
	}
	if err != nil {
		return -1, -1, err
	}
	if !found {
		return -1, -1, fmt.Errorf("user not found: %s", username)
	}
	return uid, gid, s.Err()
}
