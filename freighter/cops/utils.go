package cops

import (
	"encoding/json"
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
