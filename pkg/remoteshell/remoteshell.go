package remoteshell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/nitrous-io/nitrous-common/term"
	"github.com/nitrous-io/nitrous-common/wsconn"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
)

type TokenHandler func() (string, error)

type RemoteShell struct {
	ID           string `json:"id"`
	host         string
	containerID  string
	command      string
	isTerminal   bool // this checks if current shell is a terminal
	tokenHandler TokenHandler
}

// New creates a shell that uses WebSockets.
// FIXME Sprinkle TLS magic (i.e. https and wss://).
func New(host, containerID, command string, tokenHandler TokenHandler) *RemoteShell {
	return &RemoteShell{
		host:         host,
		containerID:  containerID,
		command:      command,
		tokenHandler: tokenHandler,
	}
}

// request a new shell id. This will create a remote shell on the specified
// container
func (s *RemoteShell) RequestShell() error {
	params := url.Values{}
	params.Set("command", s.command)
	endpoint := fmt.Sprintf("/containers/%s/shells", s.containerID)
	req, err := http.NewRequest("POST", endpoint, bytes.NewBufferString(params.Encode()))
	if err != nil {
		return err
	}
	token, err := s.tokenHandler()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = s.host

	c, err := s.getConn()
	if err != nil {
		return err
	}
	defer c.Close()
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to get a remote shell for container %s, error %v",
			s.containerID, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("Missing container %s", s.containerID)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Failed to get a remote shell for container %s, status %d",
			s.containerID, resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(s)
}

// connect stdin and stdout respectively to their remote parts
func (s *RemoteShell) Attach(fd uintptr, stdin io.Reader, stdout io.Writer) error {
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

	token, err := s.tokenHandler()
	if err != nil {
		return err
	}
	headers := http.Header{}
	headers.Add("Authorization", "Token "+token)
	wsEndpointURI := fmt.Sprintf("ws://%s/containers/%s/shells/%s/attach/ws", s.host, s.containerID, s.ID)
	ws, _, err := websocket.DefaultDialer.Dial(wsEndpointURI, headers)
	if err != nil {
		return fmt.Errorf("Failed to attach to shell for container %s: %v", s.containerID, err)
	}

	conn := wsconn.NewConnection(ws)
	defer conn.Close()

	done := make(chan error)
	go func() {
		_, err := io.Copy(stdout, conn)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			done <- nil
			return
		}
		done <- err
	}()
	go func() {
		_, err := io.Copy(conn, stdin)
		done <- err
	}()

	// monitor tty size but doesnt fail when it errors because the remote end
	// may have been terminated
	if s.isTerminal {
		s.monitorSize(fd)
	}
	return <-done
}

// Run executes a remote command inside a container in a specific host
func (s *RemoteShell) Run(fd uintptr, stdin io.Reader, stdout io.Writer) error {
	if err := s.RequestShell(); err != nil {
		return fmt.Errorf("Failed to create a shell session %v", err)
	}
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
func (s *RemoteShell) monitorSize(fd uintptr) {
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
func (s *RemoteShell) resize(fd uintptr) error {
	w, h := getSize(fd)
	params := url.Values{}
	params.Set("width", strconv.Itoa(w))
	params.Set("height", strconv.Itoa(h))
	req, err := http.NewRequest("POST",
		fmt.Sprintf("/containers/%s/shells/%s/resize", s.containerID, s.ID),
		bytes.NewBufferString(params.Encode()))
	if err != nil {
		return err
	}
	token, err := s.tokenHandler()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = s.host

	c, err := s.getConn()
	if err != nil {
		return err
	}
	defer c.Close()
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("Failed to resize session %s, err: %v", s.ID, err)
	}
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Failed to resize session %s, status %d", s.ID, resp.StatusCode)
	}
	return nil
}

// get a client connection to the remote server. The connection need to
// implements Hijacker
func (s *RemoteShell) getConn() (*httputil.ClientConn, error) {
	dial, err := net.Dial("tcp", s.host)
	if err != nil {
		return nil, fmt.Errorf("Can't connect to %s", s.host)
	}
	return httputil.NewClientConn(dial, nil), nil
}

// get current size of the client terminal
func getSize(fd uintptr) (int, int) {
	ws, err := term.GetWinsize(fd)
	if err != nil {
		return defaultWidth, defaultHeight
	}
	return int(ws.Width), int(ws.Height)
}
