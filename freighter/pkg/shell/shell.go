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

	ErrDetached = errors.New("shell: shell session detached.")
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
	containerID string
	ID          string
	execID      string
	command     string
	client      *dockerclient.Client
	tty         bool

	attachOnce sync.Once
	execChan   chan error

	stdin  io.ReadCloser
	stdinR *io.PipeReader
	stdinW *io.PipeWriter

	stdout        io.WriteCloser
	stdoutR       *io.PipeReader
	stdoutW       *io.PipeWriter
	stdoutCopyBuf *copybuffer.CopyBuffer

	killTimer *time.Timer

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
			s.Close()
			return nil, err
		}
		s.ID = strconv.FormatInt(atomic.AddInt64(&nextShellID, 1), 36)
		Shells[s.ID] = s
	}
	return s, nil
}

func (s *Shell) Destroy() {
	shellsLock.Lock()
	defer shellsLock.Unlock()
	if s.destroyed {
		return
	}
	s.destroyed = true
	s.Close()
	delete(Shells, s.ID)
}

// Close will stop the exec instance of the current shell
func (s *Shell) Close() error {
	s.stdinW.Close()
	s.stdinR.Close()
	s.stdoutW.Close()
	s.stdoutR.Close()
	return s.client.StopExec(s.execID)
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

// Attach blocks until the exec session terminates either by user or when
// timeout happens and it terminates itself.
func (s *Shell) Attach(stdin io.ReadCloser, stdout io.WriteCloser, successChan chan struct{}) error {
	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.stdout != nil {
		s.stdout.Close()
	}

	s.stdin = stdin
	s.stdout = stdout

	go func() {
		io.Copy(s.stdinW, stdin)

		// at this point, connection is disrupted
		// kill exec after 60 seconds
		if !s.destroyed {
			if s.killTimer == nil {
				s.killTimer = time.AfterFunc(ConnectionTimeout, s.Destroy)
			} else {
				s.killTimer.Reset(ConnectionTimeout)
			}
		}
	}()

	go func() {
		s.stdoutCopyBuf.WriteTo(stdout)
	}()

	prevExecChan := s.execChan
	s.execChan = make(chan error, 1)
	if prevExecChan != nil {
		prevExecChan <- ErrDetached // detach previous listeners
	}

	firstAttach := false

	s.attachOnce.Do(func() {
		firstAttach = true
		go func() {
			s.execChan <- s.client.StartExec(s.execID, dockerclient.StartExecOptions{
				Detach:       false,
				Tty:          s.tty,
				InputStream:  s.stdinR,
				OutputStream: s.stdoutW,
				ErrorStream:  s.stdoutW,
				RawTerminal:  s.tty,
				Success:      successChan,
			})
		}()
	})

	if !firstAttach {
		successChan <- struct{}{}
	}

	return <-s.execChan
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
