package shell

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/kr/pty"
	"github.com/nitrous-io/nitrous-common/term"
)

const (
	defaultTerm   = "xterm-256color"
	defaultWidth  = 80
	defaultHeight = 60

	// time to wait for process to close normally before we terminate it
	defaultFinishTimeout = 1 * time.Second
	//  time to wait for process to close before we kill it
	defaultTermTimeout = 2 * time.Second
)

var (
	ErrMissingArg  = errors.New("Missing arguments: check id and command")
	ErrInvalidTerm = errors.New("Invalid term")
)

func NewShell(id, command string) (*Shell, error) {
	if id == "" || command == "" {
		return nil, ErrMissingArg
	}
	s := &Shell{
		id:      id,
		command: command,
	}
	return s, nil
}

func OpenShell(id, command string, width, height int, env []string) (*Shell, error) {
	s, err := NewShell(id, command)
	if err != nil {
		return nil, err
	}
	if err := s.Open(width, height, env); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

type Shell struct {
	id      string
	command string
	term    *os.File
	// we use lxc-attach by default, but user can switch this to a more
	// lightweight command like bash for testing purpose
	cmd *exec.Cmd
}

func (s *Shell) Close() error {
	if s.cmd.Process != nil {
		closeProcess(s.cmd)
	}
	return s.term.Close()
}

func (s *Shell) Open(w, h int, env []string) error {
	if w == 0 {
		w = defaultWidth
	}
	if h == 0 {
		h = defaultHeight
	}
	if s.cmd == nil {
		lxcEnv := []string{fmt.Sprintf("-vTERM=%s", defaultTerm)}
		for _, e := range env {
			lxcEnv = append(lxcEnv, fmt.Sprintf("-v%s", e))
		}
		args := []string{
			"--name", s.id,
			"--clear-env",
		}
		args = append(args, lxcEnv...)
		args = append(args,
			"--",
			"sh", "-c", s.command,
		)
		s.cmd = exec.Command("lxc-attach", args...)
	}
	sh, err := pty.Start(s.cmd)
	if err != nil {
		return err
	}
	s.term = sh
	return s.Resize(w, h)
}

// AttachProtocol connects the shell with reader and writer using the protocol
// specified in `protocol.go` instead of processing reader and writer as pure
// input/output stream
//
// This allows us to multiplex several type of messages over a connec[tion
func (s *Shell) AttachProtocol(r io.Reader, w io.Writer) chan error {
	msgCh := make(chan Message)

	// setups the protocol reader and writer on top of the stream reader/writer
	stdin := NewShellReader(r, msgCh)
	stdout := NewShellWriter(w)

	// setups the pipes so we can close the copy routine properly instead of
	// using the raw terminal
	stdinR, stdinW := io.Pipe()
	go func() {
		io.Copy(stdinW, stdin)
		stdinW.Close()
	}()
	stdoutR, stdoutW := io.Pipe()
	go func() {
		io.Copy(stdoutW, s.term)
		stdoutW.Close()
	}()

	// when stdin is closed, close the stdout pipe as well. This prevent cases
	// where we hangs the shell waiting for stdout to finished while stdin is
	// closed
	promise(func() error {
		defer stdoutR.Close()
		_, err := io.Copy(s.term, stdinR)
		// if the client closed the write part of the tcp connection, we gets
		// UnexpectedEOF from the websocket, so we ignore it here
		if err == io.ErrClosedPipe || err == io.ErrUnexpectedEOF {
			err = nil
		}
		return err
	})

	// stdout
	finishStdout := promise(func() error {
		defer stdinW.Close()
		_, err := io.Copy(stdout, stdoutR)
		if err == io.ErrClosedPipe {
			err = nil
		}
		return err
	})

	return promise(func() error {
		defer s.Close()
		for {
			select {
			case msg := <-msgCh:
				switch msg := msg.(type) {
				case *ResizeMsg:
					s.Resize(msg.width, msg.height)
				case *CloseMsg:
					// return so the caller can close the connection properly
					return nil
				default:
					log.Println("shell: got unknown msg %v", msg)
				}
			case err := <-finishStdout:
				return err
			}
		}
		return nil
	})
}

func (s *Shell) Attach(stdin io.Reader, stdout io.Writer) chan error {
	errors := make(chan error, 2)

	// stdin
	go func() {
		_, err := io.Copy(s.term, stdin)
		if err == io.ErrClosedPipe {
			err = nil
		}
		errors <- err
	}()

	// stdout
	go func() {
		_, err := io.Copy(stdout, s.term)
		if err == io.ErrClosedPipe {
			err = nil
		}
		errors <- err
	}()

	return promise(func() error {
		defer s.Close()
		for i := 0; i < 2; i += 1 {
			if err := <-errors; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Shell) Resize(w, h int) error {
	if w == 0 {
		w = defaultWidth
	}
	if h == 0 {
		h = defaultHeight
	}
	return term.SetWinsize(s.fd(), &term.Winsize{
		Width:  uint16(w),
		Height: uint16(h),
	})
}

// get the file descriptor of the underlying shell.
func (s *Shell) fd() uintptr {
	return s.term.Fd()
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

// close the process by sending it SIGTERM and wait for it to complete.
// If it cant finish after timeout, kill it
func closeProcess(c *exec.Cmd) error {
	done := make(chan error, 1)
	go func() {
		done <- c.Wait()
	}()
	select {
	case <-time.After(defaultFinishTimeout):
		c.Process.Signal(syscall.SIGTERM)
	case <-time.After(defaultTermTimeout):
		return c.Process.Kill()
	case err := <-done:
		return err
	}
	return nil
}
