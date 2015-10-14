package exec

import (
	"errors"
	"time"

	"github.com/fsouza/go-dockerclient"
)

var (
	dockerHost      = "unix:///var/run/docker.sock"
	defaultUsername = "root"
	defaultTimeout  = 1 * time.Minute
)

var (
	ErrCommandTimeout = errors.New("Timeout running command")
)

type ExecOptions struct {
	Username string
	Timeout  time.Duration
}

// 1. Setup an exec instance in Running container
// 2. Run command one by one. If any commands fails, it stops and returns an error
// 3. Close the exec instance
func RunCommand(containerID string, command string, options ExecOptions) error {
	client, err := docker.NewClient(dockerHost)
	if err != nil {
		return err
	}

	username := defaultUsername
	if options.Username != "" {
		username = options.Username
	}

	timeout := defaultTimeout
	if options.Timeout != 0 {
		timeout = options.Timeout
	}

	ex, err := client.CreateExec(docker.CreateExecOptions{
		AttachStdin:  false,
		AttachStdout: true,
		AttachStderr: true,
		Container:    containerID,
		User:         username,
		Cmd:          []string{"/bin/sh", "-c", command},
	})

	if err != nil {
		return err
	}

	var errCh chan error
	done := make(chan struct{})

	go func() {
		err := client.StartExec(ex.ID, docker.StartExecOptions{
			Detach: false,
		})

		if err != nil {
			errCh <- err
		}

		done <- struct{}{}
	}()

	select {
	case <-done:
		return nil
	case err := <-errCh:
		return err
	case <-time.After(timeout):
		return ErrCommandTimeout
	}

	return nil
}
