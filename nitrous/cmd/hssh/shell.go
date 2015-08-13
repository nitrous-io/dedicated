package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/mitchellh/cli"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

var (
	termMode = "xterm-256color"
)

type shellCmd struct{}

type resizeRequest struct {
	Columns uint32
	Rows    uint32
	Width   uint32
	Height  uint32
}

func newShellCommand() cli.CommandFactory {
	return func() (cli.Command, error) {
		return &shellCmd{}, nil
	}
}

func (s *shellCmd) Run(args []string) int {
	if len(args) != 1 {
		return failf("missing host slug")
	}

	c, err := newConn(args[0])
	if err != nil {
		return failf("shell: %q", err)
	}

	session, err := c.Client.NewSession()
	if err != nil {
		return failf("shell: failed to create new session %q", err)
	}

	stdin, stdout, stderr := pipes(session)

	go io.Copy(stdin, os.Stdin)
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)

	// get the current terminal file descriptor
	fd := int(os.Stdin.Fd())
	if err := setupShell(fd, session); err != nil {
		return failf("shell: %q", err)
	}
	return 0
}

func (s *shellCmd) Help() string {
	return "hssh shell remote-host-slug"
}

func (s *shellCmd) Synopsis() string {
	return "Open a shell on the remote host"
}

// monitor sizes of the current local shell and resize the remote shell as
// necessary
func monitorSize(fd int, s *ssh.Session) {
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for _ = range sigchan {
			w, h, err := terminal.GetSize(fd)
			if err != nil {
				logrus.Errorf("failed to get terminal size %v", err)
				w, h = 80, 24
			}
			req := &resizeRequest{
				Columns: uint32(w),
				Rows:    uint32(h),
				Width:   uint32(w),
				Height:  uint32(h),
			}
			_, err = s.SendRequest("window-change", false, ssh.Marshal(req))
			if err != nil {
				logrus.Errorf("failed to send resize request %v", err)
			}
		}
	}()
}

// setup a SSH shell
func setupShell(fd int, s *ssh.Session) error {
	state, err := terminal.GetState(fd)
	if err != nil {
		return fmt.Errorf("failed to get term state %q", err)
	}
	if _, err = terminal.MakeRaw(fd); err != nil {
		return fmt.Errorf("failed to make raw terminal %q", err)
	}
	defer terminal.Restore(fd, state)

	// Request pseudo terminal with the current size
	termWidth, termHeight, err := terminal.GetSize(fd)
	if err != nil {
		return fmt.Errorf("failed to get terminal size %v", err)
	}
	if err := s.RequestPty(termMode, termHeight, termWidth, nil); err != nil {
		return fmt.Errorf("failed to request a pty via ssh %v", err)
	}

	// Start remote shell
	if err := s.Shell(); err != nil {
		return fmt.Errorf("failed to open a shell via ssh %q", err)
	}

	// setup resize hook
	go monitorSize(fd, s)

	return s.Wait()
}
