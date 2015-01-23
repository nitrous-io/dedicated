package shell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	dockerclient "github.com/fsouza/go-dockerclient"
)

const (
	defaultTerm   = "xterm-256color"
	defaultWidth  = 80
	defaultHeight = 60
)

var (
	ErrMissingArg  = errors.New("Missing arguments: check id and command")
	ErrInvalidTerm = errors.New("Invalid term")

	dockerHost = "unix:///var/run/docker.sock"
	// time to wait for process to close normally before we terminate it
	defaultFinishTimeout = 1 * time.Second
	//  time to wait for process to close before we kill it
	defaultTermTimeout = 2 * time.Second
)

func init() {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		dockerHost = host
	}
}

func NewShell(containerID, command string, noTTY bool) (*Shell, error) {
	if containerID == "" || command == "" {
		return nil, ErrMissingArg
	}
	client, err := dockerclient.NewClient(dockerHost)
	if err != nil {
		return nil, err
	}
	s := &Shell{
		containerID: containerID,
		command:     command,
		client:      client,
		isTTY:       true,
	}
	if noTTY {
		s.isTTY = false
	}
	return s, nil
}

func OpenShell(id, command string, noTTY bool) (*Shell, error) {
	s, err := NewShell(id, command, noTTY)
	if err != nil {
		return nil, err
	}
	if err := s.Open(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

type Shell struct {
	containerID string
	execID      string
	command     string
	client      *dockerclient.Client

	// if the shell requires a TTY
	isTTY bool
}

// Close will stop the exec instance of the current shell
func (s *Shell) Close() error {
	return s.client.StopExec(s.execID)
}

func (s *Shell) Open() error {
	exec, err := s.client.CreateExec(dockerclient.CreateExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          s.isTTY,
		Cmd:          []string{"sh", "-c", fmt.Sprintf("TERM=%s %s", defaultTerm, s.command)},
		Container:    s.containerID,
	})
	if err != nil {
		return err
	}
	s.execID = exec.Id
	return nil
}

func (s *Shell) Attach(stdin io.Reader, stdout io.Writer, attached chan struct{}) chan error {
	return promise(func() error {
		defer s.Close()
		return s.client.StartExec(s.execID, dockerclient.StartExecOptions{
			Detach:       false,
			Tty:          s.isTTY,
			InputStream:  stdin,
			OutputStream: stdout,
			ErrorStream:  stdout,
			RawTerminal:  s.isTTY,
			Success:      attached,
		})
	})
}

func (s *Shell) Resize(w, h int) error {
	// if this is not a tty we dont need to resize
	if !s.isTTY {
		return nil
	}
	if w == 0 {
		w = defaultWidth
	}
	if h == 0 {
		h = defaultHeight
	}
	return s.client.ResizeExecTTY(s.execID, h, w)
}

// basic promise implementation: it wraps calls a function in a goroutine,
// and returns a channel which will later return the function's return value.
func promise(f func() error) chan error {
	ch := make(chan error, 1)
	go func() {
		ch <- f()
	}()
	return ch
}
