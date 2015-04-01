package cops

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
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
	InitPid        int `json:"init_pid"`
	InitProcessPid int `json:"init_process_pid"`
}

// GetContainerPID returns the pid of the container's init process
func GetContainerPID(containerID string) (pid int, err error) {
	var oldFormat bool
	state := &containerState{}
	statePath := filepath.Join("/var/run/docker/execdriver/native", containerID, "state.json")
	stateFile, err := os.Open(statePath)
	if os.IsNotExist(err) {
		oldFormat = true
		statePath = filepath.Join("/var/lib/docker/execdriver/native", containerID, "state.json")
		stateFile, err = os.Open(statePath)
	}
	if err != nil {
		return 0, err
	}
	defer stateFile.Close()
	if err := json.NewDecoder(stateFile).Decode(state); err != nil {
		return 0, err
	}

	log.Println("got state", state)

	if oldFormat {
		pid = state.InitPid
	} else {
		pid = state.InitProcessPid
	}

	if pid == 0 {
		return 0, errors.New("invalid state.json")
	}
	return pid, nil
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
