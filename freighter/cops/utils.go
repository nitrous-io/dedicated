package cops

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Sirupsen/logrus"
)

var (
	newStateDir = "/var/run/docker/execdriver/native"
	oldStateDir = "/var/lib/docker/execdriver/native"
)

// docker's native driver stores container state in /var/lib/docker/execdriver/native/container_id/state.json
// we are only interested in InitPid because we want to join namespace of that pid
type containerState struct {
	// InitPid is used in docker < 1.6
	InitPid int `json:"init_pid"`
	// InitProcessPid is used in docker >= 1.6
	InitProcessPid int `json:"init_process_pid"`
}

// getContainerPID returns the pid of the container's init process
func getContainerPID(containerID string) (pid int, err error) {
	var oldFormat bool
	state := &containerState{}
	// in docker > 1.6, state is stored in /var/run/docker
	statePath := filepath.Join(newStateDir, containerID, "state.json")
	stateFile, err := os.Open(statePath)
	if os.IsNotExist(err) {
		oldFormat = true
		statePath = filepath.Join(oldStateDir, containerID, "state.json")
		stateFile, err = os.Open(statePath)
	}
	if err != nil {
		return 0, err
	}
	defer stateFile.Close()
	if err := json.NewDecoder(stateFile).Decode(state); err != nil {
		return 0, err
	}

	if oldFormat {
		pid = state.InitPid
	} else {
		pid = state.InitProcessPid
	}
	if pid == 0 {
		return 0, fmt.Errorf("invalid state %s", statePath)
	}
	return pid, nil
}

// getProcessOwner returns the process uid/gid
func getProcessOwner(pid int) (uid int, gid int, err error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(bufio.NewReader(f))
	for scanner.Scan() {
		l := scanner.Text()
		fields := strings.Fields(l)

		if len(fields) > 0 && strings.ToLower(fields[0]) == "uid:" {
			uid, err = strconv.Atoi(fields[1])
			if err != nil {
				return 0, 0, fmt.Errorf("invalid uid %v", err)
			}
		}
		if len(fields) > 0 && strings.ToLower(fields[0]) == "gid:" {
			gid, err = strconv.Atoi(fields[1])
			if err != nil {
				return 0, 0, fmt.Errorf("invalid gid %v", err)
			}
		}
	}
	return uid, gid, scanner.Err()
}

func sendErr(c chan *Message, orig *Message, err error) chan *Message {
	c <- &Message{
		Namespace: orig.Namespace,
		Event:     orig.Event,
		CID:       orig.CID,
		Error:     err.Error(),
	}
	close(c)
	return c
}

func sendMessage(c chan *Message, orig *Message, event string, args ...interface{}) {
	c <- &Message{
		Namespace: orig.Namespace,
		Event:     event,
		Args:      args,
	}
}

func sendFinalMessage(c chan *Message, orig *Message, event string, args ...interface{}) chan *Message {
	c <- &Message{
		Namespace: orig.Namespace,
		Event:     event,
		Args:      args,
	}
	close(c)
	return c
}

func sendFinalReply(c chan *Message, orig *Message, args []interface{}) chan *Message {
	c <- &Message{
		Namespace: orig.Namespace,
		Event:     orig.Event,
		CID:       orig.CID,
		Args:      args,
	}
	close(c)
	return c
}

// we want to remove lines like:
// "nvm_supports_source_options:.:1: no such file or directory: /dev/stdin"
// "/usr/local/opt/nvm/nvm.sh: line 1550: /dev/stdin: No such file or directory"
// They are a result of running su and I don't know why or how to not make them
// happen in the first place.
// Removing them makes stderr like/closer to what the actual command returns
func removeBogusFromStderr(s string) string {
	firstLineEnd := strings.Index(s, "\n")
	if firstLineEnd == -1 {
		return s
	}
	l := s[:firstLineEnd]
	if strings.Contains(l, "/dev/stdin: No such file or directory") ||
		strings.Contains(l, "no such file or directory: /dev/stdin") {
		s = s[firstLineEnd+1:]
	}
	return s
}

func ensureKnownShell(shell string) string {
	switch shell {
	case "/bin/sh", "/bin/bash", "/bin/zsh", "/bin/ksh":
		return shell
	default:
		return defaultShell
	}
}

func extractShellFromEtcPasswd(s, user string) string {
	for _, line := range strings.Split(s, "\n") {
		parts := strings.Split(line, ":")
		if parts[0] == user {
			return ensureKnownShell(parts[6])
		}
	}
	return ""
}

func getArgAsStrArray(arg interface{}) ([]string, bool) {
	a, ok := arg.([]interface{})
	if !ok {
		logrus.Errorf("getArgAsStrArray: %#v of type %T is not of type []interface{}", a, a)
		return nil, false
	}
	n := len(a)
	// empty array is valid
	if n == 0 {
		return nil, true
	}
	res := make([]string, n, n)
	for i := 0; i < n; i++ {
		res[i], ok = a[i].(string)
		if !ok {
			logrus.Errorf("getArgAsStrArray: a[%d] (value: '%#v', type: %T) is not of type string", i, a[i], a[i])
			return nil, false
		}
	}
	return res, true
}
