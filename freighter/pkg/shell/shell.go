package shell

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	dockerclient "github.com/fsouza/go-dockerclient"
	"github.com/nitrous-io/dedicated/freighter/pkg/copybuffer"
)

var ConnectionTimeout = 60 * time.Second

const (
	defaultTerm   = "xterm-256color"
	defaultWidth  = 80
	defaultHeight = 60
)

var (
	shellsLock  sync.Mutex
	Shells      = map[string]*Shell{}
	nextShellID = int64(0)
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

type Shell struct {
	// Exited receives the error from docker-exec when it finishes
	Exited chan error

	containerID string
	ID          string
	execID      string
	command     string
	client      *dockerclient.Client
	tty         bool

	attachOnce sync.Once
	execChan   chan error

	stdinR *io.PipeReader
	stdinW *io.PipeWriter

	stdoutR       *io.PipeReader
	stdoutW       *io.PipeWriter
	stdoutCopyBuf *copybuffer.CopyBuffer

	killTimer *time.Timer

	mu        sync.Mutex
	destroyed bool
}

func NewShell(containerID, command string, noTTY bool) (*Shell, error) {
	if containerID == "" || command == "" {
		return nil, ErrMissingArg
	}
	client, err := dockerclient.NewClient(dockerHost)
	if err != nil {
		return nil, err
	}
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	s := &Shell{
		Exited: make(chan error, 1),

		containerID: containerID,
		command:     command,
		client:      client,
		tty:         true,

		stdinR:        stdinR,
		stdinW:        stdinW,
		stdoutR:       stdoutR,
		stdoutW:       stdoutW,
		stdoutCopyBuf: &copybuffer.CopyBuffer{Src: stdoutR},
	}
	if noTTY {
		s.tty = false
	}
	return s, nil
}

func OpenShell(containerId, shellID, command string, noTTY bool) (*Shell, error) {
	shellsLock.Lock()
	defer shellsLock.Unlock()
	var s *Shell
	if shellID != "" {
		s = Shells[shellID]
		if s != nil && s.killTimer != nil {
			s.killTimer.Stop()
			s.killTimer = nil
		}
	}
	if s == nil {
		var err error
		s, err = NewShell(containerId, command, noTTY)
		if err != nil {
			return nil, err
		}
		if err := s.Open(); err != nil {
			return nil, err
		}
		s.ID = strconv.FormatInt(atomic.AddInt64(&nextShellID, 1), 36)
		Shells[s.ID] = s
	}
	return s, nil
}

// Close will stop the exec instance of the current shell. If the shell has
// been closed, it will not attempt to close it again
func (s *Shell) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destroyed {
		return nil
	}
	s.destroyed = true
	s.stdinW.Close()
	s.stdinR.Close()
	s.stdoutW.Close()
	s.stdoutR.Close()
	if s.execID != "" {
		return s.client.StopExec(s.execID)
	}
	return nil
}

func (s *Shell) Open() error {
	exec, err := s.client.CreateExec(dockerclient.CreateExecOptions{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          s.tty,
		Cmd:          []string{"sh", "-c", fmt.Sprintf("TERM=%s %s", defaultTerm, s.command)},
		Container:    s.containerID,
	})
	if err != nil {
		return err
	}
	s.execID = exec.Id
	return nil
}

// Attach connects stdin/stdout of the consumer to the docker-exec session. It
// returns whenever the client detaches from the session (i.e., the duration of
// Attach is the duration of stdin/stdout)
// The first time user calls Attach, it will create a long running goroutine
// that connects to docker-exec session and is valid throughout the shell's lifetime.
// The duration of the goroutine can be different from the duration of the
// Attach. The client should listen to .Exited channel to cleanup properly when
// the shell exits
func (s *Shell) Attach(stdin io.Reader, stdout io.Writer, success chan struct{}) error {
	errs := make(chan error, 2)
	// connect to the stdin
	go func() {
		_, err := io.Copy(s.stdinW, stdin)
		errs <- err
	}()

	// connect to stdout
	go func() {
		_, err := s.stdoutCopyBuf.WriteTo(stdout)
		errs <- err
	}()

	// create exec session
	var firstAttach bool
	s.attachOnce.Do(func() {
		firstAttach = true
		go s.attach(success)
	})
	if !firstAttach {
		// mirror the code in dockerclient.hijack which waits for success channel
		success <- struct{}{}
		<-success
	}

	for err := range errs {
		// stdout/stderr exits at this point. Schedule closing after timeout
		s.kill(ConnectionTimeout)
		if err != nil {
			return err
		}
	}
	return nil
}

// attach connects the stdin/stdout pipe of the shell to the remote docker-exec
// session. This runs for the lifetime of the shell and will return whenever
// docker-exec exits
func (s *Shell) attach(success chan struct{}) {
	s.Exited <- s.client.StartExec(s.execID, dockerclient.StartExecOptions{
		Detach:       false,
		Tty:          s.tty,
		InputStream:  s.stdinR,
		OutputStream: s.stdoutW,
		ErrorStream:  s.stdoutW,
		RawTerminal:  s.tty,
		Success:      success,
	})
	// removes the shell-id association when exits
	shellsLock.Lock()
	s.execID = "" // once StartExec returns, we dont have to stop it anymore
	s.Close()
	delete(Shells, s.ID)
	shellsLock.Unlock()
	close(s.Exited)
}

// kill stops the shell after some timeout. Calling it again will reset the
// timeout
func (s *Shell) kill(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.destroyed {
		if s.killTimer == nil {
			s.killTimer = time.AfterFunc(timeout, func() { s.Close() })
		} else {
			s.killTimer.Reset(timeout)
		}
	}
}

func (s *Shell) Resize(w, h int) error {
	// if this is not a tty we dont need to resize
	if !s.tty {
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
