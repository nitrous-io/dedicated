package cops

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// if the path is not absolute, start from the current directory
func GetPath(name string) string {
	if len(name) > 0 && name[0] == '/' {
		return name
	}
	return "./" + name
}

// docker's native driver stores container state in /var/lib/docker/execdriver/native/container_id/state.json
// we are only interested in InitPid because we want to join namespace of that pid
type containerState struct {
	InitPid int `json:"init_pid"`
}

// GetContainerPID returns the pid of the container's init process
func GetContainerPID(containerID string) (int, error) {
	state := &containerState{}
	statePath := filepath.Join("/var/lib/docker/execdriver/native", containerID, "state.json")
	stateFile, err := os.Open(statePath)
	if err != nil {
		return 0, err
	}
	defer stateFile.Close()
	if err := json.NewDecoder(stateFile).Decode(state); err != nil {
		return 0, err
	}

	return state.InitPid, nil
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
		log.Printf("getArgAsStrArray: %#v of type %T is not of type []interface{}", a, a)
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
			log.Printf("getArgAsStrArray: a[%d] (value: '%#v', type: %T) is not of type string", i, a[i], a[i])
			return nil, false
		}
	}
	return res, true
}
