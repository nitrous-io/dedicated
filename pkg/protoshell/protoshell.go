package protoshell

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/nitrous-io/nitrous-common/term"
	"github.com/nitrous-io/nitrous-common/wsconn"
)

const (
	defaultWidth  = 80
	defaultHeight = 25
)

type TokenHandler func() (string, error)

type ProtoShell struct {
	ID           string `json:"id"`
	host         string
	containerID  string
	command      string
	isTerminal   bool // this checks if the current shell is a terminal i.e. capable of resizing
	tokenHandler TokenHandler

	conn *wsconn.Conn

	in  io.Reader
	out io.Writer
}

// New creates a shell that uses WebSockets.
// FIXME Sprinkle TLS magic (i.e. https and wss://).
func New(host, containerID, command string, tokenHandler TokenHandler) *ProtoShell {
	return &ProtoShell{
		host:         host,
		containerID:  containerID,
		command:      command,
		tokenHandler: tokenHandler,
	}
}

// connect stdin and stdout respectively to their remote parts
func (s *ProtoShell) Attach(fd uintptr, stdin io.Reader, stdout io.Writer) error {
	s.isTerminal = term.IsTerminal(fd)

	if s.isTerminal {
		// restore the terminal back to old state when we finish
		var oldState *term.State
		oldState, err := term.SetRawTerminal(fd)
		if err != nil {
			return err
		}
		defer term.RestoreTerminal(fd, oldState)
	}

	// construct token header
	token, err := s.tokenHandler()
	if err != nil {
		return err
	}

	// construct params header that includes the wanted command
	params := url.Values{}
	params.Set("command", s.command)
	params.Set("token", token)
	params.Set("binary", "true")
	wsEndpointURI := fmt.Sprintf("ws://%s/containers/%s/shell?%s", s.host, s.containerID, params.Encode())

	ws, _, err := websocket.DefaultDialer.Dial(wsEndpointURI, http.Header{})
	if err != nil {
		return fmt.Errorf("Failed to attach to shell for container %s: %v", s.containerID, err)
	}

	s.conn = wsconn.NewConnection(ws)
	defer s.conn.Close()

	// wrap input/output stream with our protocol
	s.in = NewLocalInput(stdin)
	s.out = NewLocalOutput(stdout)

	done := make(chan error)
	go func() {
		_, err := io.Copy(s.out, s.conn)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			done <- nil
			return
		}
		done <- err
	}()

	go func() {
		_, err := io.Copy(s.conn, s.in)
		done <- err
	}()

	// monitor tty size but doesnt fail when it errors because the remote end
	// may have been terminated.
	//
	// We only monitor size when the current shell lives in a terminal
	if s.isTerminal {
		s.monitorSize(fd)
	}
	return <-done
}

// Run executes a remote command inside a container in a specific host
func (s *ProtoShell) Run(fd uintptr, stdin io.Reader, stdout io.Writer) error {
	if err := s.Attach(fd, stdin, stdout); err != nil {
		return fmt.Errorf("Shell detached %v", err)
	}
	return nil
}

// monitorSize resizes the local and remote shell so they have the same size
//
// There are cases where the remote-end had beeen terminated when we resizes it
// (because the process finished / terminates early because it was run wrongly),
// so we will just ignore errors here because it's probably the only source
// of error for resizing.
func (s *ProtoShell) monitorSize(fd uintptr) {
	s.resize(fd)
	sigchan := make(chan os.Signal, 1)
	signal.Notify(sigchan, syscall.SIGWINCH)
	go func() {
		for _ = range sigchan {
			s.resize(fd)
		}
	}()
}

// given the current terminal file descriptor, resize the remote terminal to
// match the local terminal size
func (s *ProtoShell) resize(fd uintptr) {
	w, h := getSize(fd)
	msg := fmt.Sprintf("w|%d %d", w, h)
	s.conn.Write([]byte(msg))
}

// get current size of the client terminal
func getSize(fd uintptr) (int, int) {
	ws, err := term.GetWinsize(fd)
	if err != nil {
		return defaultWidth, defaultHeight
	}
	return int(ws.Width), int(ws.Height)
}
